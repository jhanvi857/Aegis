package actions

import (
	"context"
	"errors"
	"testing"
)

type fakeOrchestrator struct {
	restarted string
	scaled    string
	replicas  int
	err       error
}

func (f *fakeOrchestrator) RestartContainer(_ context.Context, target string) error {
	f.restarted = target
	return f.err
}
func (f *fakeOrchestrator) ScaleService(_ context.Context, service string, replicas int) error {
	f.scaled, f.replicas = service, replicas
	return f.err
}

func TestRestartRequiresWorkingAdapter(t *testing.T) {
	t.Parallel()
	if err := NewRestartAction(nil).Execute(context.Background(), "worker", nil); err == nil {
		t.Fatal("nil adapter must not report a successful restart")
	}
	wantErr := errors.New("adapter unavailable")
	adapter := &fakeOrchestrator{err: wantErr}
	if err := NewRestartAction(adapter).Execute(context.Background(), "worker", nil); !errors.Is(err, wantErr) {
		t.Fatalf("expected adapter error, got %v", err)
	}
}

func TestScaleRequiresExplicitBoundedReplicaTarget(t *testing.T) {
	t.Parallel()
	adapter := &fakeOrchestrator{}
	action := NewScaleAction(adapter)
	for _, params := range []map[string]string{nil, {"target_replicas": "0"}, {"target_replicas": "51"}} {
		if err := action.Execute(context.Background(), "worker", params); err == nil {
			t.Fatalf("expected invalid scale target to fail: %#v", params)
		}
	}
	if adapter.scaled != "" {
		t.Fatal("invalid scale request reached adapter")
	}
	if err := action.Execute(context.Background(), "worker", map[string]string{"target_replicas": "4"}); err != nil {
		t.Fatalf("valid adapter scale failed: %v", err)
	}
	if adapter.scaled != "worker" || adapter.replicas != 4 {
		t.Fatalf("unexpected adapter request: %q %d", adapter.scaled, adapter.replicas)
	}
}

func TestUnintegratedActionsFailClosed(t *testing.T) {
	t.Parallel()
	for _, action := range []RecoveryAction{&RerouteTrafficAction{}, &FlushCacheAction{}, &IncreasePoolAction{}} {
		if err := action.Execute(context.Background(), "worker", nil); err == nil {
			t.Errorf("%s must not report success without an integration", action.Name())
		}
	}
}
