package main

import (
	"log"
	"sync"
)

type ApprovalGate struct {
	mu            sync.Mutex
	pendingPlans  map[string]bool
	approvedPlans map[string]bool
}

func NewApprovalGate() *ApprovalGate {
	return &ApprovalGate{
		pendingPlans:  make(map[string]bool),
		approvedPlans: make(map[string]bool),
	}
}

func (g *ApprovalGate) RequestApproval(planID string, riskLevel string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()

	if riskLevel == "LOW" {
		log.Printf("[approval-gate] Auto-approving low-risk plan %s", planID)
		g.approvedPlans[planID] = true
		return true
	}

	log.Printf("[approval-gate] High-risk plan %s requires human approval", planID)
	g.pendingPlans[planID] = true
	return false
}

func (g *ApprovalGate) Approve(planID string) {
	g.mu.Lock()
	defer g.mu.Unlock()

	delete(g.pendingPlans, planID)
	g.approvedPlans[planID] = true
	log.Printf("[approval-gate] Plan %s manually approved", planID)
}
