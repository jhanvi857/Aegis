package injectors

import (
	"context"
	"time"
)

type CacheDownInjector struct{}

func (i *CacheDownInjector) Name() string { return "cache_down" }

func (i *CacheDownInjector) Inject(ctx context.Context, targetNode string, duration time.Duration, params map[string]string) error {
	return CallNodeChaosInject(ctx, i.Name(), targetNode, duration, params)
}

func (i *CacheDownInjector) Revert(ctx context.Context, targetNode string) error {
	return CallNodeChaosRevert(ctx, targetNode)
}
