package injectors

import (
	"context"
	"fmt"
	"time"
)

type MQLagInjector struct{}

func (i *MQLagInjector) Name() string { return "mq_lag" }

func (i *MQLagInjector) Inject(ctx context.Context, targetNode string, duration time.Duration, params map[string]string) error {
	return fmt.Errorf("mq_lag injector not yet implemented")
}

func (i *MQLagInjector) Revert(ctx context.Context, targetNode string) error {
	return nil
}
