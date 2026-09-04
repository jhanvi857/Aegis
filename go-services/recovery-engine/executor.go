package main

import (
	"context"
	"log"

	"github.com/aegis/go-services/recovery-engine/actions"
)

type RecoveryPlanStep struct {
	ActionID   string
	ActionType string
	TargetNode string
	Parameters map[string]string
}

type Executor struct {
	registeredActions map[string]actions.RecoveryAction
}

func NewExecutor() *Executor {
	e := &Executor{
		registeredActions: make(map[string]actions.RecoveryAction),
	}
	e.registeredActions["restart"] = &actions.RestartAction{}
	e.registeredActions["scale"] = &actions.ScaleAction{}
	e.registeredActions["reroute_traffic"] = &actions.RerouteTrafficAction{}
	e.registeredActions["flush_cache"] = &actions.FlushCacheAction{}
	e.registeredActions["increase_pool"] = &actions.IncreasePoolAction{}
	return e
}

func (e *Executor) ExecuteStep(ctx context.Context, step RecoveryPlanStep) error {
	act, exists := e.registeredActions[step.ActionType]
	if !exists {
		log.Printf("[executor] Unknown recovery action: %s", step.ActionType)
		return nil
	}
	log.Printf("[executor] Executing action %s on node %s", step.ActionType, step.TargetNode)
	return act.Execute(ctx, step.TargetNode, step.Parameters)
}
