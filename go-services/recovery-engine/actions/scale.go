package actions

import (
	"context"
	"fmt"
	"log"
	"strconv"

	orchestrator "github.com/aegis/go-services/orchestrator-adapter"
)

type ScaleAction struct {
	Adapter orchestrator.OrchestratorAdapter
}

func NewScaleAction(adapter orchestrator.OrchestratorAdapter) *ScaleAction {
	return &ScaleAction{Adapter: adapter}
}

func (a *ScaleAction) Name() string { return "scale" }

func (a *ScaleAction) Execute(ctx context.Context, targetNode string, params map[string]string) error {
	if a.Adapter == nil {
		return fmt.Errorf("scale action is unavailable: no orchestrator adapter is configured")
	}
	replicas, err := strconv.Atoi(params["target_replicas"])
	if err != nil || replicas < 1 || replicas > 50 {
		return fmt.Errorf("scale action requires explicit target_replicas in range 1..50")
	}
	log.Printf("[action-scale] Scaling service %s to %d replicas", targetNode, replicas)
	if err := a.Adapter.ScaleService(ctx, targetNode, replicas); err != nil {
		log.Printf("[action-scale] Adapter returned error for %s: %v", targetNode, err)
		return err
	}
	log.Printf("[action-scale] Adapter accepted scale request for %s to %d replicas.", targetNode, replicas)
	return nil
}
