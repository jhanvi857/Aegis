package actions

import (
	"context"
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
	incStr := params["scale_increment"]
	increment := 2
	if incStr != "" {
		if val, err := strconv.Atoi(incStr); err == nil {
			increment = val
		}
	}

	targetReplicas := 3 + increment
	log.Printf("[action-scale] Scaling service %s by +%d replicas (target replicas: %d)", targetNode, increment, targetReplicas)
	if a.Adapter != nil {
		if err := a.Adapter.ScaleService(ctx, targetNode, targetReplicas); err != nil {
			log.Printf("[action-scale] Adapter returned error for %s: %v", targetNode, err)
			return err
		}
	}
	log.Printf("[action-scale] Service %s scaled to %d replicas successfully.", targetNode, targetReplicas)
	return nil
}
