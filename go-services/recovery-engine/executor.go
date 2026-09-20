package main

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	orchestrator "github.com/aegis/go-services/orchestrator-adapter"
	"github.com/aegis/go-services/recovery-engine/actions"
)

type RecoveryPlanStep struct {
	ActionID       string            `json:"action_id"`
	ActionType     string            `json:"action_type"`
	TargetNode     string            `json:"target_node"`
	ExecutionOrder int               `json:"execution_order"`
	Parameters     map[string]string `json:"parameters"`
}

type RecoveryResult struct {
	PlanID                string   `json:"plan_id"`
	Success               bool     `json:"success"`
	ExecutedActionIDs     []string `json:"executed_action_ids"`
	Details               string   `json:"details"`
	CompletedAtUnixNano   int64    `json:"completed_at_unix_nano"`
}

type Executor struct {
	adapter           orchestrator.OrchestratorAdapter
	registeredActions map[string]actions.RecoveryAction
}

func NewExecutor(adapter ...orchestrator.OrchestratorAdapter) *Executor {
	var orchAdapter orchestrator.OrchestratorAdapter
	if len(adapter) > 0 && adapter[0] != nil {
		orchAdapter = adapter[0]
	} else {
		orchAdapter = orchestrator.NewDockerAdapter()
	}

	e := &Executor{
		adapter:           orchAdapter,
		registeredActions: make(map[string]actions.RecoveryAction),
	}

	restartAct := actions.NewRestartAction(orchAdapter)
	scaleAct := actions.NewScaleAction(orchAdapter)
	rerouteAct := &actions.RerouteTrafficAction{}
	flushAct := &actions.FlushCacheAction{}
	poolAct := &actions.IncreasePoolAction{}

	// Register canonical lower-case and uppercase names
	e.RegisterAction("restart", restartAct)
	e.RegisterAction("RESTART", restartAct)
	e.RegisterAction("scale", scaleAct)
	e.RegisterAction("scale_up", scaleAct)
	e.RegisterAction("SCALE_UP", scaleAct)
	e.RegisterAction("SCALE", scaleAct)
	e.RegisterAction("reroute_traffic", rerouteAct)
	e.RegisterAction("REROUTE_TRAFFIC", rerouteAct)
	e.RegisterAction("flush_cache", flushAct)
	e.RegisterAction("FLUSH_CACHE", flushAct)
	e.RegisterAction("increase_pool", poolAct)
	e.RegisterAction("INCREASE_POOL", poolAct)
	// ISOLATE maps to reroute traffic / circuit break
	e.RegisterAction("isolate", rerouteAct)
	e.RegisterAction("ISOLATE", rerouteAct)

	return e
}

func (e *Executor) RegisterAction(name string, act actions.RecoveryAction) {
	e.registeredActions[name] = act
	e.registeredActions[strings.ToLower(name)] = act
	e.registeredActions[strings.ToUpper(name)] = act
}

func (e *Executor) ExecuteStep(ctx context.Context, step RecoveryPlanStep) error {
	act, exists := e.registeredActions[step.ActionType]
	if !exists {
		act, exists = e.registeredActions[strings.ToLower(step.ActionType)]
	}
	if !exists {
		return fmt.Errorf("unknown recovery action: %s", step.ActionType)
	}

	log.Printf("[executor] Executing step [%s] %s on node %s (order: %d)",
		step.ActionID, step.ActionType, step.TargetNode, step.ExecutionOrder)
	return act.Execute(ctx, step.TargetNode, step.Parameters)
}

func (e *Executor) ExecutePlan(ctx context.Context, planID string, steps []RecoveryPlanStep) (RecoveryResult, error) {
	executedIDs := make([]string, 0, len(steps))
	startTime := time.Now()

	for _, step := range steps {
		if err := e.ExecuteStep(ctx, step); err != nil {
			errDetails := fmt.Sprintf("Failed at step %s (%s on %s): %v", step.ActionID, step.ActionType, step.TargetNode, err)
			log.Printf("[executor] Plan %s execution failed: %s", planID, errDetails)
			return RecoveryResult{
				PlanID:              planID,
				Success:             false,
				ExecutedActionIDs:   executedIDs,
				Details:             errDetails,
				CompletedAtUnixNano: time.Now().UnixNano(),
			}, err
		}
		executedIDs = append(executedIDs, step.ActionID)
	}

	duration := time.Since(startTime)
	details := fmt.Sprintf("Successfully executed %d actions in %v", len(executedIDs), duration)
	log.Printf("[executor] Plan %s completed: %s", planID, details)

	return RecoveryResult{
		PlanID:              planID,
		Success:             true,
		ExecutedActionIDs:   executedIDs,
		Details:             details,
		CompletedAtUnixNano: time.Now().UnixNano(),
	}, nil
}
