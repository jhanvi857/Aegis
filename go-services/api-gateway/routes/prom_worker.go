package routes

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	io_prometheus_client "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	"github.com/prometheus/common/model"
	"gopkg.in/yaml.v3"
)

type PromQueryResult struct {
	Status string `json:"status"`
	Data   struct {
		ResultType string `json:"resultType"`
		Result     []struct {
			Metric map[string]string `json:"metric"`
			Value  []interface{}     `json:"value"`
		} `json:"result"`
	} `json:"data"`
}

type PrometheusWorker struct {
	promURL    string
	stopCh     chan struct{}
	mu         sync.Mutex
	running    bool
	prevTotals map[string]float64
	prevErrors map[string]float64
	httpClient *http.Client
	targets    []string
	metricMap  map[string]string
}

var (
	GlobalPromWorker *PrometheusWorker
	promWorkerOnce   sync.Once
)

func InitPrometheusWorker() {
	promWorkerOnce.Do(func() {
		promURL := strings.TrimSpace(os.Getenv("AEGIS_PROM_URL"))
		targets := scrapeTargetsFromEnv()
		if source := strings.TrimSpace(os.Getenv("AEGIS_CONFIG_FILE")); source != "" {
			targets = append(targets, targetsFromConfigSource(source)...)
		}
		if source := strings.TrimSpace(os.Getenv("AEGIS_CONFIG_URL")); source != "" {
			targets = append(targets, targetsFromConfigSource(source)...)
		}

		if promURL != "" || len(targets) > 0 {
			GlobalPromWorker = &PrometheusWorker{
				promURL:    promURL,
				stopCh:     make(chan struct{}),
				prevTotals: make(map[string]float64),
				prevErrors: make(map[string]float64),
				httpClient: &http.Client{Timeout: 3 * time.Second},
				targets:    targets,
				metricMap:  loadMetricMappings(strings.TrimSpace(os.Getenv("AEGIS_METRIC_MAPPING_FILE"))),
			}
			go GlobalPromWorker.Run()
			log.Printf("[prom-worker] Prometheus integration worker started against %s with %d scrape targets", promURL, len(targets))
		} else {
			log.Printf("[prom-worker] No Prometheus endpoint detected; OTLP push ingestion active on /api/telemetry and /v1")
		}
	})
}

func (w *PrometheusWorker) queryMetric(queryStr string) map[string]float64 {
	results := make(map[string]float64)
	endpoint := fmt.Sprintf("%s/api/v1/query?query=%s", strings.TrimRight(w.promURL, "/"), url.QueryEscape(queryStr))

	resp, err := w.httpClient.Get(endpoint)
	if err != nil || resp.StatusCode != 200 {
		if resp != nil {
			_ = resp.Body.Close()
		}
		return results
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return results
	}

	var res PromQueryResult
	if err := json.Unmarshal(body, &res); err != nil || res.Status != "success" {
		return results
	}

	for _, item := range res.Data.Result {
		svc := item.Metric["job"]
		if svc == "" {
			svc = item.Metric["service"]
		}
		if svc == "" {
			svc = item.Metric["app"]
		}
		if svc == "" {
			continue
		}
		if len(item.Value) >= 2 {
			valStr := fmt.Sprintf("%v", item.Value[1])
			if val, err := strconv.ParseFloat(valStr, 64); err == nil {
				results[svc] += val
			}
		}
	}
	return results
}

func (w *PrometheusWorker) Run() {
	interval := prometheusPollInterval()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-w.stopCh:
			return
		case <-ticker.C:
			w.pollAndIngest()
		}
	}
}

func prometheusPollInterval() time.Duration {
	seconds := getIncidentGateConfig().ScrapeIntervalSec
	if seconds <= 0 {
		seconds = 2
	}
	return time.Duration(seconds) * time.Second
}

func (w *PrometheusWorker) pollAndIngest() {
	if len(w.targets) > 0 {
		w.scrapeAndIngest()
		return
	}
	upMap := w.queryMetric(`up{job=~".+"}`)
	if len(upMap) == 0 {
		return
	}

	lagMap := w.queryMetric("consumer_lag")
	err5xxMap := w.queryMetric(`http_requests_total{status=~"5.."}`)
	totalReqMap := w.queryMetric("http_requests_total")
	latencyMap := w.queryMetric(`max by (job) (histogram_quantile(0.95, sum by (job, path, le) (rate(http_request_duration_seconds_bucket[2m])))) * 1000`)

	var batch []IngestMetricItem

	for svc, upVal := range upMap {
		isUp := upVal > 0.5
		observed := make(map[string]float64)
		if queueDepth, ok := lagMap[svc]; ok {
			observed["queue_depth"] = queueDepth
		}
		if latencyMs, ok := latencyMap[svc]; ok {
			observed["latency"] = latencyMs
		}
		currErr5xx := err5xxMap[svc]
		currTotals := totalReqMap[svc]

		// Delta error rate computation
		errorRate := 0.0
		prevTot, hasPrevTot := w.prevTotals[svc]
		prevErr := w.prevErrors[svc]
		if hasPrevTot && currTotals > prevTot {
			deltaTot := currTotals - prevTot
			deltaErr := math.Max(0, currErr5xx-prevErr)
			errorRate = math.Min(1.0, deltaErr/deltaTot)
			observed["error_rate"] = errorRate
		}
		w.prevTotals[svc] = currTotals
		w.prevErrors[svc] = currErr5xx

		status := "unknown"

		if !isUp {
			status = "offline"
		} else if errorRate >= 0.20 || observed["latency"] >= getIncidentGateConfig().LatencyThreshold {
			status = "critical"
		} else if errorRate > 0.05 {
			status = "degraded"
		} else if len(observed) > 0 {
			status = "healthy"
		}

		batch = append(batch, IngestMetricItem{
			ServiceID: svc, Status: status, Source: "prom", Observed: observed,
			ObservedTimestamps: promObservedTimestamps(observed, time.Now().UTC()),
		})
	}

	if len(batch) > 0 {
		IngestMetricsBatch(context.Background(), batch)
	}
}

func promObservedTimestamps(observed map[string]float64, timestamp time.Time) map[string]time.Time {
	result := make(map[string]time.Time, len(observed))
	for signal := range observed {
		result[signal] = timestamp
	}
	return result
}

func scrapeTargetsFromEnv() []string {
	var targets []string
	for _, value := range strings.Split(os.Getenv("AEGIS_SCRAPE_TARGETS"), ",") {
		if value = strings.TrimSpace(value); value != "" {
			targets = append(targets, value)
		}
	}
	if file := strings.TrimSpace(os.Getenv("AEGIS_SCRAPE_FILE_SD")); file != "" {
		if data, err := os.ReadFile(file); err == nil {
			var groups []struct {
				Targets []string `json:"targets"`
			}
			if json.Unmarshal(data, &groups) == nil {
				for _, group := range groups {
					targets = append(targets, group.Targets...)
				}
			}
		}
	}
	return targets
}

type configuredTarget struct {
	URL     string `json:"url" yaml:"url"`
	Target  string `json:"target" yaml:"target"`
	Metrics string `json:"metrics_url" yaml:"metrics_url"`
}

type configuredDiscovery struct {
	Targets []configuredTarget `json:"targets" yaml:"targets"`
}

type metricMappingConfig struct {
	Metrics map[string]string `json:"metrics" yaml:"metrics"`
}

func loadMetricMappings(path string) map[string]string {
	mappings := map[string]string{
		"cpu_usage": "cpu", "cpu_utilization": "cpu", "process.cpu.utilization": "cpu", "system.cpu.utilization": "cpu", "container.cpu.utilization": "cpu",
		"memory_usage_percent": "memory", "memory_utilization": "memory", "process.memory.utilization": "memory",
		"error_rate": "error_rate", "requests_per_second": "rps", "request_rate": "rps",
		"latency_ms": "latency", "p95_latency_ms": "latency", "queue_depth": "queue_depth", "db_connection_wait_ms": "db_wait",
	}
	if path == "" {
		return mappings
	}
	data, err := os.ReadFile(path)
	if err != nil {
		log.Printf("[prom-worker] metric mapping %s unavailable; using built-in names: %v", path, err)
		return mappings
	}
	var config metricMappingConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		log.Printf("[prom-worker] invalid metric mapping %s; using built-in names: %v", path, err)
		return mappings
	}
	for metric, signal := range config.Metrics {
		metric, signal = strings.ToLower(strings.TrimSpace(metric)), strings.ToLower(strings.TrimSpace(signal))
		switch signal {
		case "cpu", "memory", "error_rate", "rps", "latency", "queue_depth", "db_wait":
			mappings[metric] = signal
		default:
			log.Printf("[prom-worker] ignoring unsupported signal %q for metric %q", signal, metric)
		}
	}
	return mappings
}

func targetsFromConfigSource(source string) []string {
	var data []byte
	var err error
	if strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://") {
		client := &http.Client{Timeout: 5 * time.Second}
		resp, requestErr := client.Get(source)
		if requestErr != nil {
			log.Printf("[prom-worker] config source %s unavailable: %v", source, requestErr)
			return nil
		}
		defer resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			log.Printf("[prom-worker] config source %s returned HTTP %d", source, resp.StatusCode)
			return nil
		}
		data, err = io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	} else {
		data, err = os.ReadFile(source)
	}
	if err != nil {
		log.Printf("[prom-worker] config source %s unavailable: %v", source, err)
		return nil
	}

	var discovery configuredDiscovery
	if err := yaml.Unmarshal(data, &discovery); err != nil {
		log.Printf("[prom-worker] config source %s is not valid YAML/JSON: %v", source, err)
		return nil
	}
	targets := make([]string, 0, len(discovery.Targets))
	for _, target := range discovery.Targets {
		value := target.URL
		if value == "" {
			value = target.Metrics
		}
		if value == "" {
			value = target.Target
		}
		if strings.TrimSpace(value) != "" {
			targets = append(targets, strings.TrimSpace(value))
		}
	}
	log.Printf("[prom-worker] loaded %d targets from config source %s", len(targets), source)
	return targets
}

func (w *PrometheusWorker) scrapeAndIngest() {
	type scrapeSample struct {
		value     float64
		labels    model.LabelSet
		timestamp time.Time
	}
	byService := make(map[string]map[string]scrapeSample)
	for _, target := range w.targets {
		if !strings.HasPrefix(target, "http://") && !strings.HasPrefix(target, "https://") {
			target = "http://" + target
		}
		if !strings.HasSuffix(target, "/metrics") {
			target = strings.TrimRight(target, "/") + "/metrics"
		}
		resp, err := w.httpClient.Get(target)
		if err != nil {
			continue
		}
		parser := expfmt.TextParser{}
		families, err := parser.TextToMetricFamilies(resp.Body)
		_ = resp.Body.Close()
		if err != nil {
			continue
		}
		for name, family := range families {
			for _, metric := range family.Metric {
				labels := prometheusLabels(metric)
				service := string(labels[model.LabelName("service")])
				if service == "" {
					service = string(labels[model.LabelName("job")])
				}
				if service == "" {
					service = string(labels[model.LabelName("app")])
				}
				if service == "" {
					service = strings.Split(strings.TrimPrefix(strings.TrimPrefix(target, "http://"), "https://"), "/")[0]
				}
				if byService[service] == nil {
					byService[service] = make(map[string]scrapeSample)
				}
				value := float64(0)
				if metric.Gauge != nil {
					value = float64(metric.Gauge.GetValue())
				} else if metric.Counter != nil {
					value = float64(metric.Counter.GetValue())
				} else if metric.Untyped != nil {
					value = float64(metric.Untyped.GetValue())
				}
				timestamp := time.Now().UTC()
				if metric.TimestampMs != nil && metric.GetTimestampMs() > 0 {
					timestamp = time.UnixMilli(metric.GetTimestampMs()).UTC()
				}
				byService[service][strings.ToLower(name)] = scrapeSample{value: value, labels: labels, timestamp: timestamp}
			}

		}
	}
	batch := make([]IngestMetricItem, 0, len(byService))
	for service, metrics := range byService {
		item := IngestMetricItem{ServiceID: service, Source: "prom", Observed: make(map[string]float64), ObservedTimestamps: make(map[string]time.Time)}
		for name, sample := range metrics {
			signal := w.metricMap[name]
			value := sample.value
			if signal == "cpu" || signal == "memory" {
				if value >= 0 && value <= 1 {
					value *= 100
				}
			}
			if signal != "" {
				item.Observed[signal] = value
				item.ObservedTimestamps[signal] = sample.timestamp
			}
		}
		if len(item.Observed) == 0 {
			continue
		}
		batch = append(batch, item)
	}
	if len(batch) > 0 {
		IngestMetricsBatch(context.Background(), batch)
	}
}

func prometheusLabels(metric *io_prometheus_client.Metric) model.LabelSet {
	labels := make(model.LabelSet)
	for _, pair := range metric.Label {
		labels[model.LabelName(pair.GetName())] = model.LabelValue(pair.GetValue())
	}
	return labels
}

func DebugTargetsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"targets": scrapeTargetsFromEnv(), "prometheusURL": os.Getenv("AEGIS_PROM_URL")})
}
