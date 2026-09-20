package main

import (
	"context"
	"testing"
)

func TestExecutor_ExecuteStep(t *testing.T) {
	executor := NewExecutor()
	ctx := context.Background()

	testCases := []struct {
		actionType string
		targetNode string
		params     map[string]string
	}{
		{"restart", "node-a", map[string]string{"grace_period_sec": "2"}},
		{"RESTART", "node-b", nil},
		{"scale", "node-c", map[string]string{"scale_increment": "3"}},
		{"SCALE_UP", "node-c", nil},
		{"reroute_traffic", "gateway", map[string]string{"drain_timeout_sec": "5"}},
		{"flush_cache", "node-b", map[string]string{"async_flush": "true"}},
		{"increase_pool", "node-d", map[string]string{"pool_increment": "15"}},
		{"isolate", "node-a", nil},
	}

	for _, tc := range testCases {
		step := RecoveryPlanStep{
			ActionID:       "act-test-1",
			ActionType:     tc.actionType,
			TargetNode:     tc.targetNode,
			ExecutionOrder: 1,
			Parameters:     tc.params,
		}
		err := executor.ExecuteStep(ctx, step)
		if err != nil {
			t.Fatalf("Executor failed on action %s: %v", tc.actionType, err)
		}
	}
}

func TestExecutor_ExecutePlan(t *testing.T) {
	executor := NewExecutor()
	ctx := context.Background()

	steps := []RecoveryPlanStep{
		{
			ActionID:       "act-1",
			ActionType:     "RESTART",
			TargetNode:     "node-d",
			ExecutionOrder: 1,
			Parameters:     map[string]string{"grace_period_sec": "3"},
		},
		{
			ActionID:       "act-2",
			ActionType:     "SCALE_UP",
			TargetNode:     "node-c",
			ExecutionOrder: 2,
			Parameters:     map[string]string{"scale_increment": "2"},
		},
		{
			ActionID:       "act-3",
			ActionType:     "REROUTE_TRAFFIC",
			TargetNode:     "gateway",
			ExecutionOrder: 3,
			Parameters:     map[string]string{"drain_timeout_sec": "5"},
		},
	}

	result, err := executor.ExecutePlan(ctx, "plan-test-100", steps)
	if err != nil {
		t.Fatalf("ExecutePlan returned error: %v", err)
	}

	if !result.Success {
		t.Fatalf("Expected success true, got false")
	}
	if len(result.ExecutedActionIDs) != 3 {
		t.Fatalf("Expected 3 executed action IDs, got %d", len(result.ExecutedActionIDs))
	}
	if result.PlanID != "plan-test-100" {
		t.Fatalf("Expected planID plan-test-100, got %s", result.PlanID)
	}
}

func TestApprovalGate_Workflow(t *testing.T) {
	gate := NewApprovalGate()

	steps := []RecoveryPlanStep{
		{ActionID: "act-1", ActionType: "RESTART", TargetNode: "node-a", ExecutionOrder: 1},
	}

	// 1. Low risk auto-approval
	lowRiskApproved := gate.RequestApproval("plan-low-1", "LOW", 0.15, steps)
	if !lowRiskApproved {
		t.Fatalf("Expected low-risk plan to be auto-approved")
	}
	if !gate.IsApproved("plan-low-1") {
		t.Fatalf("Expected plan-low-1 to be marked approved")
	}

	// 2. High risk hold
	highRiskApproved := gate.RequestApproval("plan-high-1", "HIGH", 0.85, steps)
	if highRiskApproved {
		t.Fatalf("Expected high-risk plan to be held for approval, got auto-approved")
	}
	if gate.IsApproved("plan-high-1") {
		t.Fatalf("Expected plan-high-1 not to be approved yet")
	}

	// Verify pending list
	pending := gate.ListPending()
	if len(pending) != 1 || pending[0].PlanID != "plan-high-1" {
		t.Fatalf("Expected 1 pending plan 'plan-high-1', got %v", pending)
	}

	// 3. Manual Operator Approval
	ok := gate.Approve("plan-high-1")
	if !ok {
		t.Fatalf("Failed to approve plan-high-1")
	}
	if !gate.IsApproved("plan-high-1") {
		t.Fatalf("Expected plan-high-1 to be approved after manual approval")
	}
	if len(gate.ListPending()) != 0 {
		t.Fatalf("Expected 0 pending plans after approval")
	}

	// 4. Rejection workflow
	gate.RequestApproval("plan-high-2", "HIGH", 0.90, steps)
	rejected := gate.Reject("plan-high-2", "Policy violation: high blast radius")
	if !rejected {
		t.Fatalf("Failed to reject plan-high-2")
	}
	if gate.IsApproved("plan-high-2") {
		t.Fatalf("Expected plan-high-2 not to be approved")
	}
}
