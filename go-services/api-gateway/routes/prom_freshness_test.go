package routes

import (
	"context"
	"testing"
	"time"
)

func TestPrometheusPollIntervalTracksFreshnessConfig(t *testing.T) {
	t.Setenv("AEGIS_SCRAPE_INTERVAL_SEC", "3")
	t.Setenv("AEGIS_SCRAPE_INTERVAL", "7")
	if got := prometheusPollInterval(); got != 3*time.Second {
		t.Fatalf("expected canonical scrape interval of 3s, got %v", got)
	}
	t.Setenv("AEGIS_SCRAPE_INTERVAL_SEC", "")
	if got := prometheusPollInterval(); got != 7*time.Second {
		t.Fatalf("expected legacy scrape interval fallback of 7s, got %v", got)
	}
}

func TestPrometheusReachabilityMarksFreshWithoutInventingHealthOrMetrics(t *testing.T) {
	previousState := State
	previousStore := GetAuthStore()
	State = newControlPlaneState()
	SetAuthStore(nil)
	defer func() {
		State = previousState
		SetAuthStore(previousStore)
	}()

	updated := IngestMetricsBatch(context.Background(), []IngestMetricItem{{
		ServiceID: "quiet-prom-service-test",
		Source:    "prom",
		Status:    "unknown",
		Observed:  map[string]float64{},
	}})
	if updated != 1 {
		t.Fatalf("expected one service update, got %d", updated)
	}

	State.mu.RLock()
	defer State.mu.RUnlock()
	service := State.Services["quiet-prom-service-test"]
	if service == nil {
		t.Fatal("expected service to be discovered")
	}
	if service.DataStatus != "fresh" {
		t.Fatalf("expected fresh scrape status, got %q", service.DataStatus)
	}
	if service.Status != "unknown" {
		t.Fatalf("reachability alone must not claim service health, got %q", service.Status)
	}
	if len(service.ObservedSignals) != 0 || len(service.History) != 0 {
		t.Fatalf("reachability must not synthesize metric observations: signals=%v history=%v", service.ObservedSignals, service.History)
	}
	if service.LastIngestTime.IsZero() || service.LastSeen.IsZero() {
		t.Fatal("expected fresh scrape timestamps")
	}
}
