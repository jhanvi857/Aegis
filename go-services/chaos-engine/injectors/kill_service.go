package injectors

import (
	"context"
	"fmt"
	"time"
)

type KillServiceInjector struct{}

func (i *KillServiceInjector) Name() string { return "kill_service" }

func (i *KillServiceInjector) Inject(ctx context.Context, targetNode string, duration time.Duration, params map[string]string) error {
	return fmt.Errorf("kill_service injector not yet implemented (Phase 1)")
}

func (i *KillServiceInjector) Revert(ctx context.Context, targetNode string) error {
	return nil
}
