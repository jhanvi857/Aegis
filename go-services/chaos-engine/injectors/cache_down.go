package injectors

import (
	"context"
	"fmt"
	"time"
)

type CacheDownInjector struct{}

func (i *CacheDownInjector) Name() string { return "cache_down" }

func (i *CacheDownInjector) Inject(ctx context.Context, targetNode string, duration time.Duration, params map[string]string) error {
	return fmt.Errorf("cache_down injector not yet implemented")
}

func (i *CacheDownInjector) Revert(ctx context.Context, targetNode string) error {
	return nil
}
