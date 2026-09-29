package injectors

import (
	"context"
	"time"
)

type MemoryLeakInjector struct{}

func (i *MemoryLeakInjector) Name() string { return "memory_leak" }

func (i *MemoryLeakInjector) Inject(ctx context.Context, targetNode string, duration time.Duration, params map[string]string) error {
	return CallNodeChaosInject(ctx, i.Name(), targetNode, duration, params)
}

func (i *MemoryLeakInjector) Revert(ctx context.Context, targetNode string) error {
	return CallNodeChaosRevert(ctx, targetNode)
}
