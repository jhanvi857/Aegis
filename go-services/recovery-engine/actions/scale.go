package actions

import (
	"context"
	"fmt"
)

type ScaleAction struct{}

func (a *ScaleAction) Name() string { return "scale" }

func (a *ScaleAction) Execute(ctx context.Context, targetNode string, params map[string]string) error {
	return fmt.Errorf("scale action not implemented (Phase 4)")
}
