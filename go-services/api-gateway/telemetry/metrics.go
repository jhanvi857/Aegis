package telemetry

import (
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type MetricsRegistry struct {
	mu sync.RWMutex

	// Counters
	httpRequestsTotal        map[string]*int64 // key: method|path|status
	remediationsTotal        map[string]*int64 // key: action_type|status
	verificationsTotal       map[string]*int64 // key: outcome

	// Gauges
	activeDispatches         int64
	monitoredServicesTotal   map[string]*int64 // key: status

	// Histograms: request duration buckets (0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10)
	durationBuckets          []float64
	httpDurationBuckets      map[string][]int64 // key: method|path -> counts per bucket
	httpDurationSum          map[string]*float64
	httpDurationCount        map[string]*int64
}

var Metrics = NewMetricsRegistry()

func NewMetricsRegistry() *MetricsRegistry {
	return &MetricsRegistry{
		httpRequestsTotal:      make(map[string]*int64),
		remediationsTotal:      make(map[string]*int64),
		verificationsTotal:     make(map[string]*int64),
		monitoredServicesTotal: make(map[string]*int64),
		durationBuckets:        []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0, 10.0},
		httpDurationBuckets:    make(map[string][]int64),
		httpDurationSum:        make(map[string]*float64),
		httpDurationCount:      make(map[string]*int64),
	}
}

func (m *MetricsRegistry) IncHTTPRequest(method, path string, status int) {
	key := fmt.Sprintf("%s|%s|%d", method, path, status)
	m.mu.Lock()
	counter, exists := m.httpRequestsTotal[key]
	if !exists {
		var val int64
		m.httpRequestsTotal[key] = &val
		counter = &val
	}
	m.mu.Unlock()
	atomic.AddInt64(counter, 1)
}

func (m *MetricsRegistry) ObserveHTTPRequestDuration(method, path string, durationSec float64) {
	key := fmt.Sprintf("%s|%s", method, path)
	m.mu.Lock()
	defer m.mu.Unlock()

	buckets, exists := m.httpDurationBuckets[key]
	if !exists {
		buckets = make([]int64, len(m.durationBuckets))
		m.httpDurationBuckets[key] = buckets
		var sum float64
		m.httpDurationSum[key] = &sum
		var count int64
		m.httpDurationCount[key] = &count
	}

	for i, le := range m.durationBuckets {
		if durationSec <= le {
			buckets[i]++
		}
	}
	*m.httpDurationSum[key] += durationSec
	*m.httpDurationCount[key]++
}

func (m *MetricsRegistry) IncRemediation(actionType, status string) {
	key := fmt.Sprintf("%s|%s", actionType, status)
	m.mu.Lock()
	counter, exists := m.remediationsTotal[key]
	if !exists {
		var val int64
		m.remediationsTotal[key] = &val
		counter = &val
	}
	m.mu.Unlock()
	atomic.AddInt64(counter, 1)
}

func (m *MetricsRegistry) IncVerification(outcome string) {
	m.mu.Lock()
	counter, exists := m.verificationsTotal[outcome]
	if !exists {
		var val int64
		m.verificationsTotal[outcome] = &val
		counter = &val
	}
	m.mu.Unlock()
	atomic.AddInt64(counter, 1)
}

func (m *MetricsRegistry) SetActiveDispatches(count int) {
	atomic.StoreInt64(&m.activeDispatches, int64(count))
}

func (m *MetricsRegistry) SetMonitoredServices(statusCounts map[string]int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for status, count := range statusCounts {
		var val int64 = int64(count)
		m.monitoredServicesTotal[status] = &val
	}
}

// MetricsHandler serves Prometheus exposition format
func (m *MetricsRegistry) Handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		m.mu.RLock()
		defer m.mu.RUnlock()

		var b strings.Builder

		// 1. HTTP requests total
		b.WriteString("# HELP aegis_http_requests_total Total number of HTTP requests processed by Aegis control plane.\n")
		b.WriteString("# TYPE aegis_http_requests_total counter\n")
		for key, ptr := range m.httpRequestsTotal {
			parts := strings.Split(key, "|")
			if len(parts) == 3 {
				val := atomic.LoadInt64(ptr)
				b.WriteString(fmt.Sprintf("aegis_http_requests_total{method=\"%s\",path=\"%s\",status=\"%s\"} %d\n", parts[0], parts[1], parts[2], val))
			}
		}

		// 2. Active dispatches
		b.WriteString("# HELP aegis_active_dispatches Number of currently in-flight remediation dispatches.\n")
		b.WriteString("# TYPE aegis_active_dispatches gauge\n")
		b.WriteString(fmt.Sprintf("aegis_active_dispatches %d\n", atomic.LoadInt64(&m.activeDispatches)))

		// 3. Remediations total
		b.WriteString("# HELP aegis_remediations_total Total number of remediation actions executed.\n")
		b.WriteString("# TYPE aegis_remediations_total counter\n")
		for key, ptr := range m.remediationsTotal {
			parts := strings.Split(key, "|")
			if len(parts) == 2 {
				val := atomic.LoadInt64(ptr)
				b.WriteString(fmt.Sprintf("aegis_remediations_total{action_type=\"%s\",status=\"%s\"} %d\n", parts[0], parts[1], val))
			}
		}

		// 4. Verifications total
		b.WriteString("# HELP aegis_remediation_verifications_total Outcomes of remediation verification windows.\n")
		b.WriteString("# TYPE aegis_remediation_verifications_total counter\n")
		for outcome, ptr := range m.verificationsTotal {
			val := atomic.LoadInt64(ptr)
			b.WriteString(fmt.Sprintf("aegis_remediation_verifications_total{outcome=\"%s\"} %d\n", outcome, val))
		}

		// 5. Monitored services total
		b.WriteString("# HELP aegis_monitored_services_total Number of services in live topology registry by status.\n")
		b.WriteString("# TYPE aegis_monitored_services_total gauge\n")
		for status, ptr := range m.monitoredServicesTotal {
			val := atomic.LoadInt64(ptr)
			b.WriteString(fmt.Sprintf("aegis_monitored_services_total{status=\"%s\"} %d\n", status, val))
		}

		// 6. Request duration histograms
		b.WriteString("# HELP aegis_http_request_duration_seconds Latency of HTTP requests processed by Aegis.\n")
		b.WriteString("# TYPE aegis_http_request_duration_seconds histogram\n")
		for key, bucketCounts := range m.httpDurationBuckets {
			parts := strings.Split(key, "|")
			if len(parts) == 2 {
				method, path := parts[0], parts[1]
				for i, le := range m.durationBuckets {
					b.WriteString(fmt.Sprintf("aegis_http_request_duration_seconds_bucket{method=\"%s\",path=\"%s\",le=\"%.3f\"} %d\n", method, path, le, bucketCounts[i]))
				}
				count := *m.httpDurationCount[key]
				sum := *m.httpDurationSum[key]
				b.WriteString(fmt.Sprintf("aegis_http_request_duration_seconds_bucket{method=\"%s\",path=\"%s\",le=\"+Inf\"} %d\n", method, path, count))
				b.WriteString(fmt.Sprintf("aegis_http_request_duration_seconds_sum{method=\"%s\",path=\"%s\"} %.6f\n", method, path, sum))
				b.WriteString(fmt.Sprintf("aegis_http_request_duration_seconds_count{method=\"%s\",path=\"%s\"} %d\n", method, path, count))
			}
		}

		_, _ = w.Write([]byte(b.String()))
	}
}

// HTTPMetricsMiddleware instruments every HTTP route
func HTTPMetricsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		interceptor := &responseWriterInterceptor{ResponseWriter: w, statusCode: http.StatusOK}

		next.ServeHTTP(interceptor, r)

		durationSec := time.Since(start).Seconds()

		// Skip high-frequency internal scrape endpoint
		if r.URL.Path != "/metrics" {
			Metrics.IncHTTPRequest(r.Method, r.URL.Path, interceptor.statusCode)
			Metrics.ObserveHTTPRequestDuration(r.Method, r.URL.Path, durationSec)
		}
	})
}
