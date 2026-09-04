package injectors

import (
	"context"
	"fmt"
	"time"
)

type Injector interface {
	Name() string
	Inject(ctx context.Context, targetNode string, duration time.Duration, params map[string]string) error
	Revert(ctx context.Context, targetNode string) error
}

type LatencyInjector struct{}

func (i *LatencyInjector) Name() string { return "latency" }

func (i *LatencyInjector) Inject(ctx context.Context, targetNode string, duration time.Duration, params map[string]string) error {
	// Phase 1 implementation will inject network latency via tc / proxy
	return fmt.Errorf("latency injector not yet implemented (Phase 1)")
}

func (i *LatencyInjector) Revert(ctx context.Context, targetNode string) error {
	return nil
}
