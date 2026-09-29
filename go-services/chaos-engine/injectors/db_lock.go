package injectors

import (
	"context"
	"time"
)

type DBLockInjector struct{}

func (i *DBLockInjector) Name() string { return "db_lock" }

func (i *DBLockInjector) Inject(ctx context.Context, targetNode string, duration time.Duration, params map[string]string) error {
	return CallNodeChaosInject(ctx, i.Name(), targetNode, duration, params)
}

func (i *DBLockInjector) Revert(ctx context.Context, targetNode string) error {
	return CallNodeChaosRevert(ctx, targetNode)
}
