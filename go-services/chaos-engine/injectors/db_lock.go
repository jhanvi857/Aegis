package injectors

import (
	"context"
	"fmt"
	"time"
)

type DBLockInjector struct{}

func (i *DBLockInjector) Name() string { return "db_lock" }

func (i *DBLockInjector) Inject(ctx context.Context, targetNode string, duration time.Duration, params map[string]string) error {
	return fmt.Errorf("db_lock injector not yet implemented")
}

func (i *DBLockInjector) Revert(ctx context.Context, targetNode string) error {
	return nil
}
