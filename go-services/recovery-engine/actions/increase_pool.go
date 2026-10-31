package actions

import (
	"context"
	"fmt"
)

type IncreasePoolAction struct{}

func (a *IncreasePoolAction) Name() string { return "increase_pool" }

func (a *IncreasePoolAction) Execute(ctx context.Context, targetNode string, params map[string]string) error {
	return fmt.Errorf("increase_pool is unsupported: pool configuration is owned by the connected application for %q", targetNode)
}
