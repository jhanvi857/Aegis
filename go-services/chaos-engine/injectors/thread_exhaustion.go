package injectors

import (
	"context"
	"time"
)

type ThreadExhaustionInjector struct{}

func (i *ThreadExhaustionInjector) Name() string { return "thread_exhaustion" }

func (i *ThreadExhaustionInjector) Inject(ctx context.Context, targetNode string, duration time.Duration, params map[string]string) error {
	return CallNodeChaosInject(ctx, i.Name(), targetNode, duration, params)
}

func (i *ThreadExhaustionInjector) Revert(ctx context.Context, targetNode string) error {
	return CallNodeChaosRevert(ctx, targetNode)
}
