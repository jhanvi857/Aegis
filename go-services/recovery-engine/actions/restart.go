package actions

import (
	"context"
	"log"

	orchestrator "github.com/aegis/go-services/orchestrator-adapter"
)

type RecoveryAction interface {
	Name() string
	Execute(ctx context.Context, targetNode string, params map[string]string) error
}

type RestartAction struct {
	Adapter orchestrator.OrchestratorAdapter
}

func NewRestartAction(adapter orchestrator.OrchestratorAdapter) *RestartAction {
	return &RestartAction{Adapter: adapter}
}

func (a *RestartAction) Name() string { return "restart" }

func (a *RestartAction) Execute(ctx context.Context, targetNode string, params map[string]string) error {
	gracePeriod := params["grace_period_sec"]
	if gracePeriod == "" {
		gracePeriod = "5"
	}
	log.Printf("[action-restart] Initiating rolling restart for node %s (grace period: %ss)", targetNode, gracePeriod)
	if a.Adapter != nil {
		if err := a.Adapter.RestartContainer(ctx, targetNode); err != nil {
			log.Printf("[action-restart] Adapter returned error for %s: %v", targetNode, err)
			return err
		}
	}
	log.Printf("[action-restart] Node %s container restarted successfully.", targetNode)
	return nil
}
