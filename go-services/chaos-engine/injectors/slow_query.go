package injectors

import (
	"context"
	"time"
)

type SlowQueryInjector struct{}

func (i *SlowQueryInjector) Name() string { return "slow_query" }

func (i *SlowQueryInjector) Inject(ctx context.Context, targetNode string, duration time.Duration, params map[string]string) error {
	return CallNodeChaosInject(ctx, i.Name(), targetNode, duration, params)
}

func (i *SlowQueryInjector) Revert(ctx context.Context, targetNode string) error {
	return CallNodeChaosRevert(ctx, targetNode)
}
