package actions

import (
	"context"
	"fmt"
)

type IncreasePoolAction struct{}

func (a *IncreasePoolAction) Name() string { return "increase_pool" }

func (a *IncreasePoolAction) Execute(ctx context.Context, targetNode string, params map[string]string) error {
	return fmt.Errorf("increase_pool action not implemented (Phase 4)")
}
