package injectors

import (
	"context"
	"fmt"
	"time"
)

type SlowQueryInjector struct{}

func (i *SlowQueryInjector) Name() string { return "slow_query" }

func (i *SlowQueryInjector) Inject(ctx context.Context, targetNode string, duration time.Duration, params map[string]string) error {
	return fmt.Errorf("slow_query injector not yet implemented")
}

func (i *SlowQueryInjector) Revert(ctx context.Context, targetNode string) error {
	return nil
}
