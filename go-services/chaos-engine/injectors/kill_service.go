package injectors

import (
	"context"
	"time"
)

type KillServiceInjector struct{}

func (i *KillServiceInjector) Name() string { return "kill_service" }

func (i *KillServiceInjector) Inject(ctx context.Context, targetNode string, duration time.Duration, params map[string]string) error {
	return CallNodeChaosInject(ctx, i.Name(), targetNode, duration, params)
}

func (i *KillServiceInjector) Revert(ctx context.Context, targetNode string) error {
	return CallNodeChaosRevert(ctx, targetNode)
}
