package injectors

import (
	"context"
	"time"
)

type MQLagInjector struct{}

func (i *MQLagInjector) Name() string { return "mq_lag" }

func (i *MQLagInjector) Inject(ctx context.Context, targetNode string, duration time.Duration, params map[string]string) error {
	return CallNodeChaosInject(ctx, i.Name(), targetNode, duration, params)
}

func (i *MQLagInjector) Revert(ctx context.Context, targetNode string) error {
	return CallNodeChaosRevert(ctx, targetNode)
}
