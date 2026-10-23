package routes

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strings"
	"sync"
	"time"
)

// ActionPolicy defines rulebook specifications for recovery actions
type ActionPolicy struct {
	ActionType       string  `json:"actionType"`
	BaseSeverity     float64 `json:"baseSeverity"`
	BaseRiskLevel    string  `json:"baseRiskLevel"`
	ApprovalRequired bool    `json:"approvalRequired"`
	Reversible       bool    `json:"reversible"`
}

// RulebookPolicyStore holds the canonical mapping of actions and fault family escalations
var CanonicalRulebook = map[string]ActionPolicy{
	"RESTART": {
		ActionType:       "RESTART",
		BaseSeverity:     0.20,
		BaseRiskLevel:    "LOW",
		ApprovalRequired: false,
		Reversible:       false,
	},
	"FLUSH_CACHE": {
		ActionType:       "FLUSH_CACHE",
		BaseSeverity:     0.25,
		BaseRiskLevel:    "LOW",
		ApprovalRequired: false,
		Reversible:       false,
	},
	"INCREASE_POOL": {
		ActionType:       "INCREASE_POOL",
		BaseSeverity:     0.30,
		BaseRiskLevel:    "MEDIUM",
		ApprovalRequired: true,
		Reversible:       true,
	},
	"SCALE": {
		ActionType:       "SCALE_UP",
		BaseSeverity:     0.35,
		BaseRiskLevel:    "MEDIUM",
		ApprovalRequired: true,
		Reversible:       true,
	},
	"SCALE_UP": {
		ActionType:       "SCALE_UP",
		BaseSeverity:     0.35,
		BaseRiskLevel:    "MEDIUM",
		ApprovalRequired: true,
		Reversible:       true,
	},
	"REROUTE_TRAFFIC": {
		ActionType:       "REROUTE_TRAFFIC",
		BaseSeverity:     0.50,
		BaseRiskLevel:    "MEDIUM",
		ApprovalRequired: true,
		Reversible:       true,
	},
	"ISOLATE": {
		ActionType:       "ISOLATE",
		BaseSeverity:     0.80,
		BaseRiskLevel:    "HIGH",
		ApprovalRequired: true,
		Reversible:       true,
	},
}

// FaultFamilyEscalations maps primary fault families to primary and secondary escalation actions
var FaultFamilyEscalations = map[string]struct {
	Primary   string
	Secondary string
}{
	"cpu_saturation":    {Primary: "SCALE_UP", Secondary: "RESTART"},
	"memory_leak":       {Primary: "RESTART", Secondary: "SCALE_UP"},
	"network_latency":   {Primary: "REROUTE_TRAFFIC", Secondary: "RESTART"},
	"packet_loss":       {Primary: "REROUTE_TRAFFIC", Secondary: "RESTART"},
	"database":          {Primary: "INCREASE_POOL", Secondary: "RESTART"},
	"slow_query":        {Primary: "INCREASE_POOL", Secondary: "RESTART"},
	"cache":             {Primary: "FLUSH_CACHE", Secondary: "RESTART"},
	"thread_exhaustion": {Primary: "SCALE_UP", Secondary: "RESTART"},
	"crash":             {Primary: "RESTART", Secondary: "SCALE_UP"},
}

// PolicyAssessment contains the outcome of risk & approval policy evaluation
type PolicyAssessment struct {
	ActionType           string   `json:"actionType"`
	TargetServiceID      string   `json:"targetServiceId"`
	TargetTier           string   `json:"targetTier"`
	TargetCriticality    float64  `json:"targetCriticality"`
	BlastRadius          []string `json:"blastRadius"`
	BlastRadiusSize      int      `json:"blastRadiusSize"`
	RiskScore            float64  `json:"riskScore"`
	RiskLevel            string   `json:"riskLevel"`
	RequiresApproval     bool     `json:"requiresApproval"`
	CooldownRemainingSec int      `json:"cooldownRemainingSec"`
	Justification        string   `json:"justification"`
}

// NormalizeActionType standardizes string action names
func NormalizeActionType(act string) string {
	clean := strings.ToUpper(strings.TrimSpace(act))
	clean = strings.ReplaceAll(clean, "-", "_")
	switch clean {
	case "SCALE", "SCALE_UP":
		return "SCALE_UP"
	case "RESTART":
		return "RESTART"
	case "FLUSH_CACHE":
		return "FLUSH_CACHE"
	case "INCREASE_POOL", "POOL_EXPAND":
		return "INCREASE_POOL"
	case "REROUTE_TRAFFIC", "REROUTE":
		return "REROUTE_TRAFFIC"
	case "ISOLATE":
		return "ISOLATE"
	default:
		return clean
	}
}

// ComputeBlastRadius computes reachable downstream nodes via BFS on live service dependencies
func ComputeBlastRadius(targetID string) []string {
	State.mu.RLock()
	defer State.mu.RUnlock()

	visited := make(map[string]bool)
	queue := []string{targetID}
	visited[targetID] = true
	var blastRadius []string

	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]

		// Find services that depend on curr
		for id, svc := range State.Services {
			if !visited[id] {
				for _, dep := range svc.Dependencies {
					if dep == curr {
						visited[id] = true
						blastRadius = append(blastRadius, id)
						queue = append(queue, id)
						break
					}
				}
			}
		}
	}
	return blastRadius
}

// AssessPolicy computes Risk = f(action type from rulebook, target criticality/tier, blast radius, current prediction)
func AssessPolicy(actionType, targetServiceID string) PolicyAssessment {
	normAction := NormalizeActionType(actionType)
	policy, exists := CanonicalRulebook[normAction]
	if !exists {
		policy = ActionPolicy{
			ActionType:       normAction,
			BaseSeverity:     0.40,
			BaseRiskLevel:    "MEDIUM",
			ApprovalRequired: true,
			Reversible:       false,
		}
	}

	blastRadius := ComputeBlastRadius(targetServiceID)
	blastRadiusSize := len(blastRadius)

	// Determine node tier & criticality
	State.mu.RLock()
	svc, svcExists := State.Services[targetServiceID]
	criticality := 0.40
	tier := "tier-2"
	if svcExists {
		if svc.Type == "gateway" || targetServiceID == "gateway" || len(svc.Dependencies) >= 3 {
			criticality = 0.85
			tier = "tier-1-ingress"
		} else if svc.Type == "database" || strings.Contains(strings.ToLower(svc.Name), "db") {
			criticality = 0.70
			tier = "tier-1-storage"
		}
	}
	State.mu.RUnlock()

	// Blast radius amplifier (up to +0.30)
	radiusPenalty := math.Min(0.30, float64(blastRadiusSize)*0.08)

	// Criticality amplifier (up to +0.15)
	criticalityPenalty := criticality * 0.15

	// Combined risk score in [0.0, 1.0]
	riskScore := math.Round(math.Min(1.0, policy.BaseSeverity+radiusPenalty+criticalityPenalty)*100) / 100.0

	// Derive risk level
	riskLevel := "LOW"
	if riskScore >= 0.70 || normAction == "ISOLATE" {
		riskLevel = "HIGH"
	} else if riskScore >= 0.40 {
		riskLevel = "MEDIUM"
	}

	// Approval required by policy (not solely by live failure probability)
	requiresApproval := (riskLevel == "HIGH" || riskLevel == "MEDIUM" || policy.ApprovalRequired)

	// Check service cooldown
	cooldownSec := Safety.GetCooldownRemainingSec(targetServiceID)

	justification := fmt.Sprintf(
		"Policy evaluation for %s on %s: BaseSeverity=%.2f, Criticality=%.2f (%s), BlastRadius=%d nodes. RiskScore=%.2f -> RiskLevel=%s (RequiresApproval=%v)",
		normAction, targetServiceID, policy.BaseSeverity, criticality, tier, blastRadiusSize, riskScore, riskLevel, requiresApproval,
	)

	return PolicyAssessment{
		ActionType:           normAction,
		TargetServiceID:      targetServiceID,
		TargetTier:           tier,
		TargetCriticality:    criticality,
		BlastRadius:          blastRadius,
		BlastRadiusSize:      blastRadiusSize,
		RiskScore:            riskScore,
		RiskLevel:            riskLevel,
		RequiresApproval:     requiresApproval,
		CooldownRemainingSec: cooldownSec,
		Justification:        justification,
	}
}

// ─── Safety Manager ─────────────────────────────────────────────────────────

type InFlightAction struct {
	ID              string
	TargetServiceID string
	ActionType      string
	BlastRadiusSize int
	StartedAt       time.Time
}

type SafetyLimitsManager struct {
	mu                 sync.RWMutex
	ServiceCooldown    time.Duration
	MaxConcurrent      int
	BlastRadiusBudget  int
	LastActionTimes    map[string]time.Time
	InFlightActions    map[string]*InFlightAction
	VerificationWindow time.Duration
}

var Safety = &SafetyLimitsManager{
	ServiceCooldown:    30 * time.Second,
	MaxConcurrent:      2,
	BlastRadiusBudget:  5,
	LastActionTimes:    make(map[string]time.Time),
	InFlightActions:    make(map[string]*InFlightAction),
	VerificationWindow: 10 * time.Second,
}

func (s *SafetyLimitsManager) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.LastActionTimes = make(map[string]time.Time)
	s.InFlightActions = make(map[string]*InFlightAction)
}

func (s *SafetyLimitsManager) GetCooldownRemainingSec(serviceID string) int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	last, exists := s.LastActionTimes[serviceID]
	if !exists {
		return 0
	}
	elapsed := time.Since(last)
	if elapsed >= s.ServiceCooldown {
		return 0
	}
	return int(math.Ceil((s.ServiceCooldown - elapsed).Seconds()))
}

func (s *SafetyLimitsManager) CanExecute(serviceID string, blastRadiusSize int) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// 1. Check service cooldown
	if last, exists := s.LastActionTimes[serviceID]; exists {
		elapsed := time.Since(last)
		if elapsed < s.ServiceCooldown {
			remaining := int(math.Ceil((s.ServiceCooldown - elapsed).Seconds()))
			return fmt.Errorf("safety limit: service '%s' is in cooldown period (%ds remaining). Minimum %ds required between remediation attempts",
				serviceID, remaining, int(s.ServiceCooldown.Seconds()))
		}
	}

	// 2. Check max concurrent actions limit
	if len(s.InFlightActions) >= s.MaxConcurrent {
		return fmt.Errorf("safety limit: maximum concurrent remediation actions (%d) currently active in verification window. Wait for in-flight tasks to complete",
			s.MaxConcurrent)
	}

	// 3. Check global blast radius budget
	currentBlastRadius := 0
	for _, act := range s.InFlightActions {
		currentBlastRadius += act.BlastRadiusSize
	}
	if currentBlastRadius+blastRadiusSize > s.BlastRadiusBudget {
		return fmt.Errorf("safety limit: global blast-radius budget exceeded. In-flight blast radius (%d) + target blast radius (%d) exceeds cluster limit (%d)",
			currentBlastRadius, blastRadiusSize, s.BlastRadiusBudget)
	}

	return nil
}

func (s *SafetyLimitsManager) RegisterDispatch(id, serviceID, actionType string, blastRadiusSize int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.LastActionTimes[serviceID] = time.Now()
	s.InFlightActions[id] = &InFlightAction{
		ID:              id,
		TargetServiceID: serviceID,
		ActionType:      actionType,
		BlastRadiusSize: blastRadiusSize,
		StartedAt:       time.Now(),
	}
}

func (s *SafetyLimitsManager) CompleteDispatch(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.InFlightActions, id)
}

func (s *SafetyLimitsManager) ActiveDispatchCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.InFlightActions)
}

func (s *SafetyLimitsManager) IsInCooldown(serviceID string) bool {
	return s.GetCooldownRemainingSec(serviceID) > 0
}

// ─── HTTP Policy Handlers ───────────────────────────────────────────────────

type PolicyCheckRequest struct {
	ActionType      string `json:"actionType"`
	TargetServiceID string `json:"targetServiceId"`
}

// PolicyCheckHandler allows manual UI components to query policy, risk badge, and approval requirement before dispatch
func PolicyCheckHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var req PolicyCheckRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"Invalid request payload"}`, http.StatusBadRequest)
		return
	}

	if req.TargetServiceID == "" {
		http.Error(w, `{"error":"targetServiceId is required"}`, http.StatusBadRequest)
		return
	}

	assessment := AssessPolicy(req.ActionType, req.TargetServiceID)
	_ = json.NewEncoder(w).Encode(assessment)
}

// StatusCheckHandler retrieves live verification status for an execution ID
func StatusCheckHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	id := strings.TrimPrefix(r.URL.Path, "/api/recovery/status/")

	store := GetAuthStore()
	if store == nil {
		http.Error(w, `{"error":"Store unavailable"}`, http.StatusInternalServerError)
		return
	}

	exec, err := store.GetRecoveryExecution(r.Context(), id)
	if err != nil {
		http.Error(w, `{"error":"Recovery execution record not found"}`, http.StatusNotFound)
		return
	}

	_ = json.NewEncoder(w).Encode(exec)
}
