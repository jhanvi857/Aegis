package injectors

import (
	"context"
	"fmt"
	"time"
)

type CPUStressInjector struct{}

func (i *CPUStressInjector) Name() string { return "cpu_stress" }

func (i *CPUStressInjector) Inject(ctx context.Context, targetNode string, duration time.Duration, params map[string]string) error {
	return fmt.Errorf("cpu_stress injector not yet implemented (Phase 1)")
}

func (i *CPUStressInjector) Revert(ctx context.Context, targetNode string) error {
	return nil
}
