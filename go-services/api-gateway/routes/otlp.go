package routes

import (
	"compress/gzip"
	"encoding/hex"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/aegis/go-services/api-gateway/storage"
	metricscollectorpb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	logscollectorpb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	tracecollectorpb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"
	logspb "go.opentelemetry.io/proto/otlp/logs/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

func ingestAuthorized(r *http.Request) bool {
	cfg := GetConfig()
	if cfg == nil || cfg.IngestToken == "" {
		return true
	}
	token := r.Header.Get("Authorization")
	if strings.HasPrefix(strings.ToLower(token), "bearer ") {
		token = strings.TrimSpace(token[7:])
	}
	if token == "" {
		token = r.Header.Get("X-Aegis-Ingest-Token")
	}
	return token == cfg.IngestToken
}

func OTLPAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !ingestAuthorized(r) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func decodeOTLP(r *http.Request, msg proto.Message) error {
	var reader io.Reader = r.Body
	if strings.Contains(strings.ToLower(r.Header.Get("Content-Encoding")), "gzip") {
		zr, err := gzip.NewReader(r.Body)
		if err != nil {
			return err
		}
		defer zr.Close()
		reader = zr
	}
	body, err := io.ReadAll(io.LimitReader(reader, 32<<20))
	if err != nil {
		return err
	}
	if strings.Contains(strings.ToLower(r.Header.Get("Content-Type")), "json") {
		return protojson.Unmarshal(body, msg)
	}
	return proto.Unmarshal(body, msg)
}

func IngestOTLPTracesHandler(w http.ResponseWriter, r *http.Request) {
	req := new(tracecollectorpb.ExportTraceServiceRequest)
	if err := decodeOTLP(r, req); err != nil {
		http.Error(w, "invalid OTLP trace payload", http.StatusBadRequest)
		return
	}
	type resourceSpan struct {
		span *tracepb.Span
		service, namespace, clusterID string
		resourceAttributes map[string]string
	}
	resourceSpans := make([]resourceSpan, 0)
	for _, batch := range req.ResourceSpans {
		service := resourceServiceName(batch.Resource)
		for _, scope := range batch.ScopeSpans {
			for _, span := range scope.Spans {
				resourceSpans = append(resourceSpans, resourceSpan{
					span: span, service: service,
					namespace: resourceAttributeString(batch.Resource, "service.namespace"),
					clusterID: resourceAttributeString(batch.Resource, "k8s.cluster.name"),
					resourceAttributes: resourceStringAttributes(batch.Resource),
				})
			}

		}
	}
	if len(resourceSpans) == 0 {
		w.WriteHeader(http.StatusOK)
		return
	}
	spanServices := make(map[string]string, len(resourceSpans))
	for _, item := range resourceSpans {
		if item.service != "" {
			spanServices[hex.EncodeToString(item.span.TraceId)+":"+hex.EncodeToString(item.span.SpanId)] = item.service
		}
	}
	spans := make([]IngestSpanItem, 0, len(resourceSpans))
	for _, item := range resourceSpans {
		span := item.span
		traceID, spanID, parentSpanID := hex.EncodeToString(span.TraceId), hex.EncodeToString(span.SpanId), hex.EncodeToString(span.ParentSpanId)
		source, target := item.service, ""
		// A server span whose remote parent is in this batch gives us an exact
		// service-to-service relationship. Prefer it over server.address or
		// net.peer.name, which commonly identify a hostname/IP rather than the
		// caller's service identity.
		if span.Kind == tracepb.Span_SPAN_KIND_SERVER && parentSpanID != "" {
			if parentService := spanServices[traceID+":"+parentSpanID]; parentService != "" && parentService != item.service {
				source, target = parentService, item.service
			}
		}
		if target == "" {
			target = attributeString(span.Attributes, "peer.service")
		}
		if target == "" {
			target = attributeString(span.Attributes, "server.address")
		}
		if target == "" {
			target = attributeString(span.Attributes, "net.peer.name")
		}
		if target == "" {
			target = attributeString(span.Attributes, "rpc.service")
		}
		if target == "" {
			target = attributeString(span.Attributes, "http.host")
		}
		if idx := strings.Index(target, ":"); idx != -1 {
			target = target[:idx]
		}
		spans = append(spans, IngestSpanItem{
			TraceID: traceID, SpanID: spanID, ParentSpanID: parentSpanID, ServiceID: item.service,
			SourceServiceID: source, TargetServiceID: target,
			OperationName: span.Name, StartTimeUnixNano: int64(span.StartTimeUnixNano), EndTimeUnixNano: int64(span.EndTimeUnixNano),
			DurationMs: float64(span.EndTimeUnixNano-span.StartTimeUnixNano) / 1e6,
			StatusCode: otlpStatusCode(span.Status.Code), Attributes: attributesMap(span.Attributes),
			Namespace: item.namespace, ClusterID: item.clusterID,
			ResourceAttributes: item.resourceAttributes,
		})
	}

	rememberRecentSpans(spans)
	ingestSpanItems(r, spans)
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"partialSuccess":{}}`))
}

func IngestOTLPLogsHandler(w http.ResponseWriter, r *http.Request) {
	req := new(logscollectorpb.ExportLogsServiceRequest)
	if err := decodeOTLP(r, req); err != nil {
		http.Error(w, "invalid OTLP logs payload", http.StatusBadRequest)
		return
	}

	dropped := 0
	entries := make([]LogEntry, 0)
	services := make(map[string]IngestMetricItem)
	for _, resourceLogs := range req.ResourceLogs {
		service := resourceServiceName(resourceLogs.Resource)
		if service != "" {
			namespace, clusterID := resourceAttributeString(resourceLogs.Resource, "service.namespace"), resourceAttributeString(resourceLogs.Resource, "k8s.cluster.name")
			services[service] = IngestMetricItem{ServiceID: service, Source: "otlp-logs", Namespace: namespace, ClusterID: clusterID, ResourceAttributes: resourceStringAttributes(resourceLogs.Resource), Observed: map[string]float64{}}
		}
		for _, scope := range resourceLogs.ScopeLogs {
			for _, record := range scope.LogRecords {
				if service == "" {
					dropped++
					continue
				}
				message := anyValueString(record.Body)
				if len(message) > 8192 { message = message[:8192] }
				level := strings.ToUpper(record.SeverityText)
				if level == "" { level = severityName(record.SeverityNumber) }
				timestamp := time.Now().UTC()
				if record.TimeUnixNano > 0 { timestamp = time.Unix(0, int64(record.TimeUnixNano)).UTC() } else if record.ObservedTimeUnixNano > 0 { timestamp = time.Unix(0, int64(record.ObservedTimeUnixNano)).UTC() }
				entries = append(entries, LogEntry{
					ID: hex.EncodeToString(record.GetTraceId()) + "-" + hex.EncodeToString(record.GetSpanId()) + "-" + strconv.FormatInt(timestamp.UnixNano(), 10), Timestamp: timestamp.Format(time.RFC3339Nano),
					ServiceID: service, ServiceName: service, Level: level, Message: message,
					TraceID: hex.EncodeToString(record.GetTraceId()), SpanID: hex.EncodeToString(record.GetSpanId()),
					ResourceAttributes: resourceStringAttributes(resourceLogs.Resource),
				})
			}
		}
	}
	for _, service := range services { EnsureDiscoveredService(r.Context(), service.ServiceID, service.Source, service.Namespace, service.ClusterID, service.ResourceAttributes) }
	State.mu.Lock()
	State.Logs = append(State.Logs, entries...)
	if len(State.Logs) > 2000 { State.Logs = State.Logs[len(State.Logs)-2000:] }
	State.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	if dropped > 0 {
		_, _ = w.Write([]byte(`{"partialSuccess":{"rejectedLogRecords":` + strconv.Itoa(dropped) + `}}`))
		return
	}
	_, _ = w.Write([]byte(`{"partialSuccess":{}}`))
}

func anyValueString(value *commonpb.AnyValue) string {
	if value == nil { return "" }
	switch v := value.Value.(type) {
	case *commonpb.AnyValue_StringValue: return v.StringValue
	case *commonpb.AnyValue_BoolValue: return strconv.FormatBool(v.BoolValue)
	case *commonpb.AnyValue_IntValue: return strconv.FormatInt(v.IntValue, 10)
	case *commonpb.AnyValue_DoubleValue: return strconv.FormatFloat(v.DoubleValue, 'g', -1, 64)
	default:
		data, err := protojson.Marshal(value)
		if err != nil { return "" }
		return string(data)
	}
}

func severityName(number logspb.SeverityNumber) string {
	switch {
	case number >= logspb.SeverityNumber_SEVERITY_NUMBER_FATAL: return "FATAL"
	case number >= logspb.SeverityNumber_SEVERITY_NUMBER_ERROR: return "ERROR"
	case number >= logspb.SeverityNumber_SEVERITY_NUMBER_WARN: return "WARN"
	case number >= logspb.SeverityNumber_SEVERITY_NUMBER_INFO: return "INFO"
	case number >= logspb.SeverityNumber_SEVERITY_NUMBER_DEBUG: return "DEBUG"
	default: return "UNSPECIFIED"
	}
}

func otlpStatusCode(code tracepb.Status_StatusCode) int {
	if code == tracepb.Status_STATUS_CODE_ERROR {
		return 500
	}
	return 200
}

func IngestOTLPMetricsHandler(w http.ResponseWriter, r *http.Request) {
	req := new(metricscollectorpb.ExportMetricsServiceRequest)
	if err := decodeOTLP(r, req); err != nil {
		http.Error(w, "invalid OTLP metrics payload", http.StatusBadRequest)
		return
	}
	metrics := make([]IngestMetricItem, 0)
	byService := make(map[string]*IngestMetricItem)
	for _, batch := range req.ResourceMetrics {
		service := resourceServiceName(batch.Resource)
		if service == "" {
			continue
		}
		item := byService[service]
		if item == nil {
			item = &IngestMetricItem{ServiceID: service, Source: "otlp", Observed: make(map[string]float64), ObservedTimestamps: make(map[string]time.Time), Namespace: resourceAttributeString(batch.Resource, "service.namespace"), ClusterID: resourceAttributeString(batch.Resource, "k8s.cluster.name"), ResourceAttributes: resourceStringAttributes(batch.Resource)}
			byService[service] = item
		}
		for _, scope := range batch.ScopeMetrics {
			for _, metric := range scope.Metrics {
				observed := metricToObserved(metric)
				timestamp := metricTimestamp(metric)
				for key, value := range observed {
					if previous := item.ObservedTimestamps[key]; !timestamp.IsZero() && !previous.IsZero() && timestamp.Before(previous) { continue }
					item.Observed[key] = value
					if !timestamp.IsZero() { item.ObservedTimestamps[key] = timestamp }
				}
			}
		}
	}
	for _, item := range byService {
		if len(item.Observed) == 0 {
			EnsureDiscoveredService(r.Context(), item.ServiceID, item.Source, item.Namespace, item.ClusterID, item.ResourceAttributes)
			continue
		}
		metrics = append(metrics, *item)
	}
	IngestMetricsBatch(r.Context(), metrics)
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"partialSuccess":{}}`))
}

func metricTimestamp(metric *metricspb.Metric) time.Time {
	if metric == nil { return time.Time{} }
	var latest uint64
	switch data := metric.Data.(type) {
	case *metricspb.Metric_Gauge:
		for _, point := range data.Gauge.DataPoints { if point.TimeUnixNano > latest { latest = point.TimeUnixNano } }
	case *metricspb.Metric_Sum:
		for _, point := range data.Sum.DataPoints { if point.TimeUnixNano > latest { latest = point.TimeUnixNano } }
	case *metricspb.Metric_Histogram:
		for _, point := range data.Histogram.DataPoints { if point.TimeUnixNano > latest { latest = point.TimeUnixNano } }
	}
	if latest == 0 { return time.Time{} }
	return time.Unix(0, int64(latest)).UTC()
}

func resourceServiceName(resource *resourcepb.Resource) string {
	if resource == nil {
		return ""
	}
	return attributeString(resource.Attributes, "service.name")
}

func resourceAttributeString(resource *resourcepb.Resource, key string) string {
	if resource == nil {
		return ""
	}
	return attributeString(resource.Attributes, key)
}

func resourceStringAttributes(resource *resourcepb.Resource) map[string]string {
	result := make(map[string]string)
	if resource == nil { return result }
	for _, attribute := range resource.Attributes {
		if attribute == nil || attribute.Value == nil { continue }
		value := anyValueString(attribute.Value)
		if value != "" { result[attribute.Key] = value }
	}
	return result
}

func attributeString(attrs []*commonpb.KeyValue, key string) string {
	for _, attr := range attrs {
		if attr.Key == key && attr.Value != nil {
			return attr.Value.GetStringValue()
		}
	}
	return ""
}

func attributesMap(attrs []*commonpb.KeyValue) map[string]interface{} {
	out := make(map[string]interface{})
	for _, attr := range attrs {
		if attr.Value != nil {
			out[attr.Key] = attr.Value.GetStringValue()
		}
	}
	return out
}

func metricToObserved(metric *metricspb.Metric) map[string]float64 {
	observed := make(map[string]float64)
	if metric == nil {
		return observed
	}
	name := strings.ToLower(metric.Name)
	var value float64
	switch data := metric.Data.(type) {
	case *metricspb.Metric_Gauge:
		value = numberDataPointValue(latestNumberPoint(data.Gauge.DataPoints))
	case *metricspb.Metric_Sum:
		value = numberDataPointValue(latestNumberPoint(data.Sum.DataPoints))
	case *metricspb.Metric_Histogram:
		if dp := latestHistogramPoint(data.Histogram.DataPoints); dp != nil {
			value = dp.GetSum()
			if dp.Count > 0 {
				value /= float64(dp.Count)
			}
		}
	}
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return observed
	}
	switch name {
	case "cpu_usage", "cpu.utilization", "process.cpu.utilization", "system.cpu.utilization", "container.cpu.utilization":
		if value >= 0 && value <= 1 {
			value *= 100
		}
		observed["cpu"] = value
	case "memory_usage_percent", "memory.utilization", "process.memory.utilization", "system.memory.utilization", "container.memory.utilization":
		if value >= 0 && value <= 1 {
			value *= 100
		}
		observed["memory"] = value
	case "latency_ms", "p95_latency_ms", "http.server.request.duration_ms":
		observed["latency"] = value
	case "http.server.request.duration":
		// OTLP HTTP duration histograms conventionally use seconds; the average
		// is retained here because raw histogram buckets are not a p95 estimate.
		switch strings.ToLower(strings.TrimSpace(metric.Unit)) {
		case "s":
			value *= 1000
		case "ms":
		default:
			return observed
		}
		observed["latency"] = value
	case "error_rate", "http.server.error_rate", "http.server.request.error_rate":
		observed["error_rate"] = value
	case "requests_per_second", "request_rate", "throughput":
		observed["rps"] = value
	case "queue_depth", "messaging.consumer.queue.depth":
		observed["queue_depth"] = value
	case "db_connection_wait_ms":
		observed["db_wait"] = value
	}
	return observed
}

func latestNumberPoint(points []*metricspb.NumberDataPoint) *metricspb.NumberDataPoint {
	var latest *metricspb.NumberDataPoint
	for _, point := range points {
		if point != nil && (latest == nil || point.TimeUnixNano >= latest.TimeUnixNano) { latest = point }
	}
	return latest
}

func latestHistogramPoint(points []*metricspb.HistogramDataPoint) *metricspb.HistogramDataPoint {
	var latest *metricspb.HistogramDataPoint
	for _, point := range points {
		if point != nil && (latest == nil || point.TimeUnixNano >= latest.TimeUnixNano) { latest = point }
	}
	return latest
}

func numberDataPointValue(point *metricspb.NumberDataPoint) float64 {
	if point == nil { return math.NaN() }
	switch value := point.Value.(type) {
	case *metricspb.NumberDataPoint_AsDouble:
		return value.AsDouble
	case *metricspb.NumberDataPoint_AsInt:
		return float64(value.AsInt)
	default:
		return math.NaN()
	}
}

func ingestSpanItems(r *http.Request, spans []IngestSpanItem) {
	for _, span := range spans {
		ingestSpanItem(r, span)
	}
}

func ingestSpanItem(r *http.Request, span IngestSpanItem) {
	source, target := span.SourceServiceID, span.TargetServiceID
	if source == "" {
		source = span.ServiceID
	}
	if source == "" {
		return
	}
	span.SourceServiceID, span.TargetServiceID = source, target
	// Reuse the existing edge discovery path without requiring an authenticated UI session.
	store := GetAuthStore()
	if store == nil {
		return
	}
	for _, id := range []string{source, target} {
		if id == "" {
			continue
		}
		if existing, _ := store.GetService(r.Context(), id); existing == nil {
			namespace, clusterID := span.Namespace, span.ClusterID
			if namespace == "" { namespace = "default" }
			if clusterID == "" { clusterID = "default" }
			_ = store.UpsertService(r.Context(), &storage.TopologyService{ID: id, Name: id, Type: "service", Namespace: namespace, ClusterID: clusterID, Tier: "tier-2", Replicas: 1, MaxReplicas: 10, MonitoringMode: "observe", Source: "otlp"})
		}
	}
	if service, _ := store.GetService(r.Context(), source); service != nil {
		now := time.Now().UTC()
		service.LastTelemetryTime = &now
		service.LastSeen = &now
		service.Source = "otlp"
		service.DataStatus = "fresh"
		_ = store.UpsertService(r.Context(), service)
	}
	if target == "" || target == source {
		return
	}
	edge, err := store.AutoDiscoverSpanEdge(r.Context(), source, target, "default", "cluster-primary", span.DurationMs, span.StatusCode >= 400)
	if err != nil || edge == nil {
		return
	}
	State.mu.Lock()
	ensureServiceLocked(source)
	ensureServiceLocked(target)
	if edge.State != "rejected" {
		svc := State.Services[source]
		for _, dep := range svc.Dependencies {
			if dep == target {
				State.mu.Unlock()
				return
			}
		}
		svc.Dependencies = append(svc.Dependencies, target)
	}
	State.mu.Unlock()
}

func ensureServiceLocked(id string) {
	if _, ok := State.Services[id]; ok {
		return
	}
	State.Services[id] = &Microservice{ID: id, Name: id, Type: "service", Status: "unknown", DataStatus: "unknown", Replicas: 1, MaxReplicas: 10, Dependencies: []string{}, History: []ServiceMetric{}, LastIngestTime: time.Time{}}
	State.ServiceOrder = append(State.ServiceOrder, id)
}
