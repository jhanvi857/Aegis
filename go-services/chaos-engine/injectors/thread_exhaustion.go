package injectors

import (
	"context"
	"fmt"
	"time"
)

type ThreadExhaustionInjector struct{}

func (i *ThreadExhaustionInjector) Name() string { return "thread_exhaustion" }

func (i *ThreadExhaustionInjector) Inject(ctx context.Context, targetNode string, duration time.Duration, params map[string]string) error {
	return fmt.Errorf("thread_exhaustion injector not yet implemented")
}

func (i *ThreadExhaustionInjector) Revert(ctx context.Context, targetNode string) error {
	return nil
}
