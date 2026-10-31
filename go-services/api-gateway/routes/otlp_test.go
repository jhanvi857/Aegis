package routes

import (
	"bytes"
	"context"
	"net/http/httptest"
	"time"
	"testing"
	"github.com/aegis/go-services/api-gateway/storage"

	metricscollectorpb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	"google.golang.org/protobuf/proto"
)

func TestOTLPMetricMappingUsesExplicitSupportedSignals(t *testing.T) {
	fraction := 0.25
	metric := &metricspb.Metric{
		Name: "process.cpu.utilization",
		Data: &metricspb.Metric_Gauge{Gauge: &metricspb.Gauge{DataPoints: []*metricspb.NumberDataPoint{{Value: &metricspb.NumberDataPoint_AsDouble{AsDouble: fraction}}}}},
	}
	got := metricToObserved(metric)
	if got["cpu"] != 25 {
		t.Fatalf("expected CPU fraction normalized to 25%%, got %#v", got)
	}
	unsupported := metricToObserved(&metricspb.Metric{Name: "process.cpu.time", Data: metric.Data})
	if len(unsupported) != 0 {
		t.Fatalf("cumulative CPU time must not be mistaken for utilization: %#v", unsupported)
	}
}

func TestOTLPMetricsMergePerServiceAndPreserveExplicitZero(t *testing.T) {
	setupTestState()
	before := State.Services["node-a"].Latency
	IngestMetricsBatch(context.Background(), []IngestMetricItem{{
		ServiceID: "node-a", Source: "otlp", Observed: map[string]float64{"cpu": 0},
	}})
	State.mu.RLock()
	svc := State.Services["node-a"]
	cpu, latency := svc.CPU, svc.Latency
	State.mu.RUnlock()
	if cpu != 0 {
		t.Fatalf("explicit zero CPU value was lost: %v", cpu)
	}
	if latency != before {
		t.Fatalf("unobserved latency was overwritten: got %v want %v", latency, before)
	}
}

func TestOTLPHandlerDiscoversGenericServiceFromResourceIdentity(t *testing.T) {
	InitState()
	value := 0.2
	request := &metricscollectorpb.ExportMetricsServiceRequest{ResourceMetrics: []*metricspb.ResourceMetrics{{
		Resource: &resourcepb.Resource{Attributes: []*commonpb.KeyValue{
			{Key: "service.name", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "generic-worker"}}},
			{Key: "service.namespace", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "test-space"}}},
		}},
		ScopeMetrics: []*metricspb.ScopeMetrics{{Metrics: []*metricspb.Metric{{
			Name: "process.cpu.utilization",
			Data: &metricspb.Metric_Gauge{Gauge: &metricspb.Gauge{DataPoints: []*metricspb.NumberDataPoint{{Value: &metricspb.NumberDataPoint_AsDouble{AsDouble: value}}}}},
		}}}},
	}}}
	payload, err := proto.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("POST", "/v1/metrics", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/x-protobuf")
	response := httptest.NewRecorder()
	IngestOTLPMetricsHandler(response, req)
	if response.Code != 200 {
		t.Fatalf("unexpected status %d: %s", response.Code, response.Body.String())
	}
	State.mu.RLock()
	svc := State.Services["generic-worker"]
	State.mu.RUnlock()
	if svc == nil {
		t.Fatal("service.name did not create a service")
	}
	if svc.CPU != 20 {
		t.Fatalf("expected CPU usage normalized to 20%%, got %v", svc.CPU)
	}
	if svc.Namespace != "test-space" {
		t.Fatalf("service namespace not preserved: %q", svc.Namespace)
	}
	if svc.ResourceAttributes["service.namespace"] != "test-space" { t.Fatalf("resource attributes not retained: %#v", svc.ResourceAttributes) }
}

func TestDiscoveredResourceAttributesPersistInRegistry(t *testing.T) {
	store := storage.NewMemoryStore()
	SetAuthStore(store)
	InitState()
	attrs := map[string]string{"service.name":"batch-consumer", "deployment.environment":"staging"}
	EnsureDiscoveredService(context.Background(), "batch-consumer", "otlp", "commerce", "cluster-7", attrs)
	registered, err := store.GetService(context.Background(), "batch-consumer")
	if err != nil { t.Fatal(err) }
	if registered.ResourceAttributes["deployment.environment"] != "staging" { t.Fatalf("resource attrs not persisted: %#v", registered.ResourceAttributes) }
}

func TestMetricIngestionRejectsOutOfOrderSignalSamples(t *testing.T) {
	setupTestState()
	first := time.Now().UTC().Add(-time.Second)
	older := first.Add(-time.Second)
	IngestMetricsBatch(context.Background(), []IngestMetricItem{{
		ServiceID: "node-a", Source: "otlp", Observed: map[string]float64{"cpu": 40},
		ObservedTimestamps: map[string]time.Time{"cpu": first},
	}})
	IngestMetricsBatch(context.Background(), []IngestMetricItem{{
		ServiceID: "node-a", Source: "otlp", Observed: map[string]float64{"cpu": 95},
		ObservedTimestamps: map[string]time.Time{"cpu": older},
	}})
	State.mu.RLock()
	got := State.Services["node-a"].CPU
	observedAt := State.Services["node-a"].SignalLastSeen["cpu"]
	State.mu.RUnlock()
	if got != 40 { t.Fatalf("out-of-order sample overwrote current value: %v", got) }
	if !observedAt.Equal(first) { t.Fatalf("signal timestamp changed: got %s want %s", observedAt, first) }
}
