package actions

import (
	"context"
	"fmt"
)

type FlushCacheAction struct{}

func (a *FlushCacheAction) Name() string { return "flush_cache" }

func (a *FlushCacheAction) Execute(ctx context.Context, targetNode string, params map[string]string) error {
	return fmt.Errorf("flush_cache action not implemented (Phase 4)")
}
