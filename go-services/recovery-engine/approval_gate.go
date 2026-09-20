package main

import (
	"fmt"
	"log"
	"strings"
	"sync"
	"time"
)

type PendingPlanInfo struct {
	PlanID       string             `json:"plan_id"`
	RiskLevel    string             `json:"risk_level"`
	RiskScore    float64            `json:"risk_score"`
	Steps        []RecoveryPlanStep `json:"steps"`
	RequestedAt  time.Time          `json:"requested_at"`
	Status       string             `json:"status"` // pending, approved, rejected
	RejectReason string             `json:"reject_reason,omitempty"`
}

type ApprovalGate struct {
	mu           sync.RWMutex
	pendingPlans map[string]*PendingPlanInfo
}

func NewApprovalGate() *ApprovalGate {
	return &ApprovalGate{
		pendingPlans: make(map[string]*PendingPlanInfo),
	}
}

// RequestApproval checks whether a plan requires human approval or is auto-approved.
// Returns true if auto-approved, false if blocked waiting for approval.
func (g *ApprovalGate) RequestApproval(
	planID string,
	riskLevel string,
	riskScore float64,
	steps []RecoveryPlanStep,
) bool {
	g.mu.Lock()
	defer g.mu.Unlock()

	normalizedLevel := strings.ToUpper(riskLevel)

	// Low risk and medium risk with score < 0.70 are auto-approved
	if normalizedLevel == "LOW" || (normalizedLevel == "MEDIUM" && riskScore < 0.70) {
		log.Printf("[approval-gate] Auto-approving %s risk plan %s (score: %.2f)", normalizedLevel, planID, riskScore)
		g.pendingPlans[planID] = &PendingPlanInfo{
			PlanID:      planID,
			RiskLevel:   normalizedLevel,
			RiskScore:   riskScore,
			Steps:       steps,
			RequestedAt: time.Now(),
			Status:      "approved",
		}
		return true
	}

	// High-risk plans hold pending human authorization
	log.Printf("[approval-gate] HIGH RISK plan %s held pending human operator approval (score: %.2f)", planID, riskScore)
	g.pendingPlans[planID] = &PendingPlanInfo{
		PlanID:      planID,
		RiskLevel:   normalizedLevel,
		RiskScore:   riskScore,
		Steps:       steps,
		RequestedAt: time.Now(),
		Status:      "pending",
	}
	return false
}

// Approve manually authorizes a pending plan.
func (g *ApprovalGate) Approve(planID string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()

	plan, exists := g.pendingPlans[planID]
	if !exists {
		log.Printf("[approval-gate] Approval requested for unknown plan %s", planID)
		return false
	}

	plan.Status = "approved"
	log.Printf("[approval-gate] Plan %s manually APPROVED by operator.", planID)
	return true
}

// Reject cancels a pending plan with a reason.
func (g *ApprovalGate) Reject(planID string, reason string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()

	plan, exists := g.pendingPlans[planID]
	if !exists {
		return false
	}

	plan.Status = "rejected"
	plan.RejectReason = reason
	log.Printf("[approval-gate] Plan %s REJECTED: %s", planID, reason)
	return true
}

// IsApproved checks if the plan is authorized for execution.
func (g *ApprovalGate) IsApproved(planID string) bool {
	g.mu.RLock()
	defer g.mu.RUnlock()

	plan, exists := g.pendingPlans[planID]
	if !exists {
		return false
	}
	return plan.Status == "approved"
}

// ListPending returns all plans currently awaiting approval.
func (g *ApprovalGate) ListPending() []PendingPlanInfo {
	g.mu.RLock()
	defer g.mu.RUnlock()

	result := make([]PendingPlanInfo, 0)
	for _, p := range g.pendingPlans {
		if p.Status == "pending" {
			result = append(result, *p)
		}
	}
	return result
}

// GetPlan retrieves plan info by ID.
func (g *ApprovalGate) GetPlan(planID string) (*PendingPlanInfo, error) {
	g.mu.RLock()
	defer g.mu.RUnlock()

	plan, exists := g.pendingPlans[planID]
	if !exists {
		return nil, fmt.Errorf("plan not found: %s", planID)
	}
	copyPlan := *plan
	return &copyPlan, nil
}
