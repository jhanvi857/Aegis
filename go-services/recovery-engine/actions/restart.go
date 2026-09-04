package actions

import (
	"context"
	"fmt"
)

type RecoveryAction interface {
	Name() string
	Execute(ctx context.Context, targetNode string, params map[string]string) error
}

type RestartAction struct{}

func (a *RestartAction) Name() string { return "restart" }

func (a *RestartAction) Execute(ctx context.Context, targetNode string, params map[string]string) error {
	// Phase 4 will execute container/pod restart via orchestrator adapter
	return fmt.Errorf("restart action not implemented (Phase 4)")
}
