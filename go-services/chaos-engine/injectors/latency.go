package injectors

import (
	"context"
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
	return CallNodeChaosInject(ctx, i.Name(), targetNode, duration, params)
}

func (i *LatencyInjector) Revert(ctx context.Context, targetNode string) error {
	return CallNodeChaosRevert(ctx, targetNode)
}
