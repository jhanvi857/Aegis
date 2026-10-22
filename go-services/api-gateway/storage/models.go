package storage

import (
	"strings"
	"time"
)

const (
	RoleViewer   = "viewer"
	RoleOperator = "operator"
	RoleAdmin    = "admin"
)

func RoleLevel(role string) int {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case RoleAdmin:
		return 3
	case RoleOperator:
		return 2
	case RoleViewer:
		return 1
	default:
		return 0
	}
}

func HasMinRole(userRole, requiredRole string) bool {
	return RoleLevel(userRole) >= RoleLevel(requiredRole)
}

type User struct {
	ID                  string     `json:"id"`
	Name                string     `json:"name"`
	Email               string     `json:"email"`
	PasswordHash        string     `json:"-"`
	Salt                string     `json:"-"`
	Role                string     `json:"role"`
	FailedLoginAttempts int        `json:"failedLoginAttempts,omitempty"`
	LockedUntil         *time.Time `json:"lockedUntil,omitempty"`
	CreatedAt           time.Time  `json:"createdAt"`
	UpdatedAt           time.Time  `json:"updatedAt"`
}

type Session struct {
	ID        string     `json:"id"`
	UserID    string     `json:"userId"`
	ExpiresAt time.Time  `json:"expiresAt"`
	RevokedAt *time.Time `json:"revokedAt,omitempty"`
	CreatedAt time.Time  `json:"createdAt"`
}

type APIKey struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	KeyPrefix string     `json:"keyPrefix"`
	KeyHash   string     `json:"-"`
	Scopes    string     `json:"scopes"`
	Role      string     `json:"role"`
	CreatedBy string     `json:"createdBy"`
	ExpiresAt *time.Time `json:"expiresAt,omitempty"`
	RevokedAt *time.Time `json:"revokedAt,omitempty"`
	CreatedAt time.Time  `json:"createdAt"`
}

func (k *APIKey) HasScope(scope string) bool {
	if k.Scopes == "*" {
		return true
	}
	parts := strings.Split(k.Scopes, ",")
	for _, p := range parts {
		if strings.TrimSpace(p) == scope || strings.TrimSpace(p) == "*" {
			return true
		}
	}
	return false
}

type AuditLogEntry struct {
	ID              int64     `json:"id"`
	Timestamp       time.Time `json:"timestamp"`
	ActorID         string    `json:"actorId"`
	ActorEmail      string    `json:"actorEmail"`
	ActorRole       string    `json:"actorRole"`
	Action          string    `json:"action"`
	TargetServiceID string    `json:"targetServiceId,omitempty"`
	RiskLevel       string    `json:"riskLevel,omitempty"`
	ApprovalStatus  string    `json:"approvalStatus,omitempty"`
	Payload         string    `json:"payload,omitempty"`
	ResultStatus    string    `json:"resultStatus"` // SUCCESS, FAILED, BLOCKED
	ErrorMessage    string    `json:"errorMessage,omitempty"`
	IPAddress       string    `json:"ipAddress,omitempty"`
}

type CallerContext struct {
	ActorID   string
	Email     string
	Role      string
	IsAPIKey  bool
	APIKeyObj *APIKey
}

type RecoveryExecution struct {
	ID               string                 `json:"id"`
	PlanID           string                 `json:"planId"`
	ActionID         string                 `json:"actionId"`
	ActionType       string                 `json:"actionType"`
	TargetServiceID  string                 `json:"targetServiceId"`
	Status           string                 `json:"status"` // Pending, Verifying, Verified, Ineffective, RolledBack
	RiskLevel        string                 `json:"riskLevel"`
	RiskScore        float64                `json:"riskScore"`
	RequiresApproval bool                   `json:"requiresApproval"`
	Approver         string                 `json:"approver,omitempty"`
	DryRun           bool                   `json:"dryRun"`
	IdempotencyKey   string                 `json:"idempotencyKey,omitempty"`
	DurationMs       int64                  `json:"durationMs,omitempty"`
	DurationStr      string                 `json:"duration"`
	MetricsBefore    map[string]interface{} `json:"metricsBefore,omitempty"`
	MetricsAfter     map[string]interface{} `json:"metricsAfter,omitempty"`
	RiskBefore       float64                `json:"riskBefore"`
	RiskAfter        float64                `json:"riskAfter"`
	EscalatedAction  string                 `json:"escalatedAction,omitempty"`
	ErrorMessage     string                 `json:"errorMessage,omitempty"`
	StartedAt        time.Time              `json:"startedAt"`
	CompletedAt      *time.Time             `json:"completedAt,omitempty"`
}

type TopologyService struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Type         string    `json:"type"` // gateway, service, database, cache, queue
	Namespace    string    `json:"namespace"`
	ClusterID    string    `json:"clusterId"`
	Tier         string    `json:"tier"` // tier-1-ingress, tier-1-storage, tier-2, tier-3
	Capabilities string    `json:"capabilities"`
	Owner        string    `json:"owner"`
	Version      string    `json:"version"`
	Replicas     int       `json:"replicas"`
	MaxReplicas  int       `json:"maxReplicas"`
	Status       string    `json:"status"` // healthy, degraded, critical, unknown
	CPU          float64   `json:"cpu"`
	Memory       float64   `json:"memory"`
	Latency      float64   `json:"latency"`
	RPS          float64   `json:"rps"`
	ErrorRate    float64   `json:"errorRate"`
	QueueDepth   int       `json:"queueDepth"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

type TopologyEdge struct {
	ID               string    `json:"id"`
	SourceServiceID  string    `json:"sourceServiceId"`
	TargetServiceID  string    `json:"targetServiceId"`
	Relationship     string    `json:"relationship"`
	Namespace        string    `json:"namespace"`
	ClusterID        string    `json:"clusterId"`
	Confidence       float64   `json:"confidence"`
	IsManualOverride bool      `json:"isManualOverride"`
	CallsPerSec      float64   `json:"callsPerSec"`
	LatencyMs        float64   `json:"latencyMs"`
	ErrorRate        float64   `json:"errorRate"`
	SampleCount      int64     `json:"sampleCount"`
	CreatedAt        time.Time `json:"createdAt"`
	UpdatedAt        time.Time `json:"updatedAt"`
}

type TopologyVersion struct {
	ID          string                 `json:"id"`
	Version     int                    `json:"version"`
	Description string                 `json:"description"`
	Snapshot    map[string]interface{} `json:"snapshot"`
	CreatedBy   string                 `json:"createdBy"`
	CreatedAt   time.Time              `json:"createdAt"`
}
