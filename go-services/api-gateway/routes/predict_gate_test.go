package routes

import (
	"testing"
	"time"
)

func TestIsMetricBreachedRequiresObservedSignal(t *testing.T) {
	cfg := getIncidentGateConfig()
	svc := &Microservice{
		Status: "critical",
		CPU:    cfg.CPUThreshold + 10,
	}

	if isMetricBreached(svc, cfg) {
		t.Fatal("status and an unobserved metric must not open the incident gate")
	}

	svc.SignalLastSeen = map[string]time.Time{
		"cpu": time.Now().UTC(),
	}
	if !isMetricBreached(svc, cfg) {
		t.Fatal("a fresh observed threshold breach should open the metric gate")
	}
}
