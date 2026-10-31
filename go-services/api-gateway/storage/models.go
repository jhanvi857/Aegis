package storage

import (
	"encoding/json"
	"fmt"
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
	MustChangePassword  bool       `json:"mustChangePassword,omitempty"`
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
	ResultStatus    string    `json:"resultStatus"` // DISPATCHED, VERIFIED, UNVERIFIED, INEFFECTIVE, SUCCESS, FAILED, BLOCKED
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
	Status           string                 `json:"status"` // Pending, Dispatching, Verifying, Verified, Unverified, Ineffective, Failed, RolledBack
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

type TelemetrySource struct {
	Type            string            `json:"type"` // prometheus | kubernetes | agent_push
	BaseURL         string            `json:"baseUrl,omitempty"`
	JobSelector     string            `json:"jobSelector,omitempty"`
	LabelSelector   string            `json:"labelSelector,omitempty"`
	PromQLTemplates map[string]string `json:"promqlTemplates,omitempty"`
	ClusterID       string            `json:"clusterId,omitempty"`
	Namespace       string            `json:"namespace,omitempty"`
	WorkloadKind    string            `json:"workloadKind,omitempty"`
	WorkloadName    string            `json:"workloadName,omitempty"`
	ServiceID       string            `json:"serviceId,omitempty"`
	APIKeyRef       string            `json:"apiKeyRef,omitempty"`
	AuthToken       string            `json:"authToken,omitempty"` // encrypted at rest, redacted on output
	AuthUsername    string            `json:"authUsername,omitempty"`
	AuthPassword    string            `json:"authPassword,omitempty"` // encrypted at rest, redacted on output
}

type RecoveryBinding struct {
	Runtime        string   `json:"runtime"`        // kubernetes | docker
	Target         string   `json:"target"`         // namespace/deployment or container/project
	AllowedActions []string `json:"allowedActions"` // scale, restart, increase_pool, flush_cache, reroute
}

type TopologyService struct {
	ID                string           `json:"id"`
	Name              string           `json:"name"`
	Type              string           `json:"type"` // gateway, service, database, cache, queue
	Namespace         string           `json:"namespace"`
	ClusterID         string           `json:"clusterId"`
	Tier              string           `json:"tier"` // tier-1-ingress, tier-1-storage, tier-2, tier-3
	Capabilities      string           `json:"capabilities"`
	Owner             string           `json:"owner"`
	Version           string           `json:"version"`
	Replicas          int              `json:"replicas"`
	MaxReplicas       int              `json:"maxReplicas"`
	Status            string           `json:"status"` // unconnected | healthy_data | no_data | stale
	CPU               float64          `json:"cpu"`
	Memory            float64          `json:"memory"`
	Latency           float64          `json:"latency"`
	RPS               float64          `json:"rps"`
	ErrorRate         float64          `json:"errorRate"`
	QueueDepth        int              `json:"queueDepth"`
	MonitoringMode    string           `json:"monitoringMode"` // ignore | observe | recommend | auto_remediate
	TelemetrySource   *TelemetrySource `json:"telemetrySource,omitempty"`
	RecoveryBinding   *RecoveryBinding `json:"recoveryBinding,omitempty"`
	Description       string           `json:"description,omitempty"`
	Tags              []string         `json:"tags,omitempty"`
	LastTelemetryTime *time.Time       `json:"lastTelemetryTime,omitempty"`
	Source            string           `json:"source,omitempty"`
	LastSeen          *time.Time       `json:"lastSeen,omitempty"`
	DataStatus        string           `json:"dataStatus,omitempty"`
	ResourceAttributes map[string]string `json:"resourceAttributes,omitempty"`
	CreatedAt         time.Time        `json:"createdAt"`
	UpdatedAt         time.Time        `json:"updatedAt"`
}

func (s *TopologyService) UnmarshalJSON(data []byte) error {
	type serviceAlias TopologyService
	aux := struct {
		*serviceAlias
		Tier     json.RawMessage `json:"tier"`
		Workload string          `json:"workload"`
	}{
		serviceAlias: (*serviceAlias)(s),
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	if len(aux.Tier) > 0 && string(aux.Tier) != "null" {
		var tierName string
		if err := json.Unmarshal(aux.Tier, &tierName); err == nil {
			s.Tier = tierName
		} else {
			var tierNumber int
			if err := json.Unmarshal(aux.Tier, &tierNumber); err != nil {
				return fmt.Errorf("tier must be a string or integer: %w", err)
			}
			s.Tier = fmt.Sprintf("tier-%d", tierNumber)
		}
	}
	if s.Type == "" {
		s.Type = aux.Workload
	}
	return nil
}

func (s *TopologyService) ComputeStatus() string {
	if s.MonitoringMode == "ignore" {
		return "unconnected"
	}
	if s.TelemetrySource == nil || strings.TrimSpace(s.TelemetrySource.Type) == "" {
		return "unconnected"
	}
	if s.LastTelemetryTime == nil || s.LastTelemetryTime.IsZero() {
		return "no_data"
	}
	if time.Since(*s.LastTelemetryTime) > 5*time.Minute {
		return "stale"
	}
	return "healthy_data"
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
	Source           string    `json:"source"`   // manual | suggested | discovered
	State            string    `json:"state"`    // active | suggested | rejected
	EdgeType         string    `json:"edgeType"` // sync | async | db | cache
	CreatedBy        string    `json:"createdBy"`
	CreatedAt        time.Time `json:"createdAt"`
	UpdatedAt        time.Time `json:"updatedAt"`
}

func (e *TopologyEdge) UnmarshalJSON(data []byte) error {
	type edgeAlias TopologyEdge
	aux := struct {
		*edgeAlias
		From string `json:"from"`
		To   string `json:"to"`
		Type string `json:"type"`
	}{
		edgeAlias: (*edgeAlias)(e),
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	if e.SourceServiceID == "" {
		e.SourceServiceID = aux.From
	}
	if e.TargetServiceID == "" {
		e.TargetServiceID = aux.To
	}
	if e.EdgeType == "" {
		e.EdgeType = aux.Type
	}
	if e.Relationship == "" {
		e.Relationship = "dependency"
	}
	return nil
}

type TopologyVersion struct {
	ID          string                 `json:"id"`
	Version     int                    `json:"version"`
	Description string                 `json:"description"`
	Snapshot    map[string]interface{} `json:"snapshot"`
	CreatedBy   string                 `json:"createdBy"`
	CreatedAt   time.Time              `json:"createdAt"`
}
