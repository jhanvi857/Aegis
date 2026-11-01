package actions

import (
	"context"
	"fmt"
	"log"

	orchestrator "github.com/aegis/go-services/orchestrator-adapter"
)

type IncreasePoolAction struct {
	Adapter orchestrator.OrchestratorAdapter
}

func NewIncreasePoolAction(adapter orchestrator.OrchestratorAdapter) *IncreasePoolAction {
	return &IncreasePoolAction{Adapter: adapter}
}

func (a *IncreasePoolAction) Name() string { return "increase_pool" }

func (a *IncreasePoolAction) Execute(ctx context.Context, targetNode string, params map[string]string) error {
	if a.Adapter == nil {
		return fmt.Errorf("increase_pool is unsupported: pool configuration is owned by the connected application for %q", targetNode)
	}
	log.Printf("[action-increase_pool] Initiating connection pool scale for node %s", targetNode)
	if err := a.Adapter.IncreasePool(ctx, targetNode); err != nil {
		log.Printf("[action-increase_pool] Adapter returned error for %s: %v", targetNode, err)
		return err
	}
	log.Printf("[action-increase_pool] Node %s pool increased successfully.", targetNode)
	return nil
}
