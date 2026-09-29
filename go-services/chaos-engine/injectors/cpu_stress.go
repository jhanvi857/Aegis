package injectors

import (
	"context"
	"time"
)

type CPUStressInjector struct{}

func (i *CPUStressInjector) Name() string { return "cpu_stress" }

func (i *CPUStressInjector) Inject(ctx context.Context, targetNode string, duration time.Duration, params map[string]string) error {
	return CallNodeChaosInject(ctx, i.Name(), targetNode, duration, params)
}

func (i *CPUStressInjector) Revert(ctx context.Context, targetNode string) error {
	return CallNodeChaosRevert(ctx, targetNode)
}
