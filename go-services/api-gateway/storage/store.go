package storage

import (
	"context"
	"time"
)

type Store interface {
	// User operations
	GetUserByEmail(ctx context.Context, email string) (*User, error)
	GetUserByID(ctx context.Context, id string) (*User, error)
	CreateUser(ctx context.Context, user *User) error
	UpdateUserRole(ctx context.Context, id string, role string) error
	UpdateUserPassword(ctx context.Context, id, passwordHash, salt string, mustChangePassword bool) error
	UpdateUserLoginAttempt(ctx context.Context, email string, failedAttempts int, lockedUntil *time.Time) error
	ListUsers(ctx context.Context) ([]*User, error)
	CountUsers(ctx context.Context) (int, error)

	// Session operations
	CreateSession(ctx context.Context, session *Session) error
	GetSession(ctx context.Context, token string) (*Session, *User, error)
	RevokeSession(ctx context.Context, token string) error
	RevokeUserSessions(ctx context.Context, userID string) error

	// Machine API Key operations
	CreateAPIKey(ctx context.Context, key *APIKey) error
	GetAPIKeyByHash(ctx context.Context, hash string) (*APIKey, error)
	ListAPIKeys(ctx context.Context) ([]*APIKey, error)
	RevokeAPIKey(ctx context.Context, id string) error

	// Audit Log operations (Append-only)
	AppendAuditLog(ctx context.Context, entry *AuditLogEntry) error
	ListAuditLogs(ctx context.Context, limit int) ([]*AuditLogEntry, error)

	// Recovery Execution & Verification operations (Phase 2)
	CreateRecoveryExecution(ctx context.Context, exec *RecoveryExecution) error
	UpdateRecoveryExecution(ctx context.Context, exec *RecoveryExecution) error
	GetRecoveryExecution(ctx context.Context, id string) (*RecoveryExecution, error)
	GetRecoveryExecutionByIdempotencyKey(ctx context.Context, key string) (*RecoveryExecution, error)
	ListRecoveryExecutions(ctx context.Context, limit int) ([]*RecoveryExecution, error)

	// Dynamic Service Registry & Topology operations (Phase 3)
	UpsertService(ctx context.Context, svc *TopologyService) error
	GetService(ctx context.Context, id string) (*TopologyService, error)
	DeleteService(ctx context.Context, id string) error
	ListServices(ctx context.Context, namespace, clusterID string) ([]*TopologyService, error)

	UpsertEdge(ctx context.Context, edge *TopologyEdge) error
	GetEdgeByID(ctx context.Context, id string) (*TopologyEdge, error)
	DeleteEdge(ctx context.Context, sourceID, targetID, namespace, clusterID string) error
	ListEdges(ctx context.Context, namespace, clusterID string) ([]*TopologyEdge, error)
	ListSuggestions(ctx context.Context, namespace, clusterID string) ([]*TopologyEdge, error)

	AutoDiscoverSpanEdge(ctx context.Context, sourceID, targetID, namespace, clusterID string, latencyMs float64, isError bool) (*TopologyEdge, error)

	CreateTopologyVersion(ctx context.Context, version *TopologyVersion) error
	GetLatestTopologyVersion(ctx context.Context) (*TopologyVersion, error)
	ListTopologyVersions(ctx context.Context, limit int) ([]*TopologyVersion, error)

	// Lifecycle
	Close() error
}
