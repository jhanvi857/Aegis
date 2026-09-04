package injectors

import (
	"context"
	"fmt"
	"time"
)

type MemoryLeakInjector struct{}

func (i *MemoryLeakInjector) Name() string { return "memory_leak" }

func (i *MemoryLeakInjector) Inject(ctx context.Context, targetNode string, duration time.Duration, params map[string]string) error {
	return fmt.Errorf("memory_leak injector not yet implemented (Phase 1)")
}

func (i *MemoryLeakInjector) Revert(ctx context.Context, targetNode string) error {
	return nil
}
