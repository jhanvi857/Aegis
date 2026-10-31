package storage

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"
)

var (
	ErrNotFound = errors.New("record not found")
	ErrConflict = errors.New("record already exists")
)

type MemoryStore struct {
	mu            sync.RWMutex
	usersByID     map[string]*User
	usersByEm     map[string]*User
	sessions      map[string]*Session
	apiKeys       map[string]*APIKey // keyHash -> APIKey
	apiKeysID     map[string]*APIKey // id -> APIKey
	auditLogs     []*AuditLogEntry
	auditSeq      int64
	recoveryByID  map[string]*RecoveryExecution
	recoveryByKey map[string]*RecoveryExecution
	recoveryList  []*RecoveryExecution
	servicesByID  map[string]*TopologyService
	edgesByKey    map[string]*TopologyEdge
	edgesByID     map[string]*TopologyEdge
	versionList   []*TopologyVersion
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		usersByID:     make(map[string]*User),
		usersByEm:     make(map[string]*User),
		sessions:      make(map[string]*Session),
		apiKeys:       make(map[string]*APIKey),
		apiKeysID:     make(map[string]*APIKey),
		auditLogs:     make([]*AuditLogEntry, 0),
		recoveryByID:  make(map[string]*RecoveryExecution),
		recoveryByKey: make(map[string]*RecoveryExecution),
		recoveryList:  make([]*RecoveryExecution, 0),
		servicesByID:  make(map[string]*TopologyService),
		edgesByKey:    make(map[string]*TopologyEdge),
		edgesByID:     make(map[string]*TopologyEdge),
		versionList:   make([]*TopologyVersion, 0),
	}
}

func (m *MemoryStore) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	u, exists := m.usersByEm[email]
	if !exists {
		return nil, ErrNotFound
	}
	cp := *u
	return &cp, nil
}

func (m *MemoryStore) GetUserByID(ctx context.Context, id string) (*User, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	u, exists := m.usersByID[id]
	if !exists {
		return nil, ErrNotFound
	}
	cp := *u
	return &cp, nil
}

func (m *MemoryStore) CreateUser(ctx context.Context, user *User) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.usersByEm[user.Email]; exists {
		return ErrConflict
	}
	cp := *user
	if cp.CreatedAt.IsZero() {
		cp.CreatedAt = time.Now().UTC()
	}
	cp.UpdatedAt = cp.CreatedAt
	m.usersByID[user.ID] = &cp
	m.usersByEm[user.Email] = &cp
	return nil
}

func (m *MemoryStore) UpdateUserRole(ctx context.Context, id string, role string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	u, exists := m.usersByID[id]
	if !exists {
		return ErrNotFound
	}
	u.Role = role
	u.UpdatedAt = time.Now().UTC()
	return nil
}

func (m *MemoryStore) UpdateUserPassword(ctx context.Context, id, passwordHash, salt string, mustChangePassword bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	u, exists := m.usersByID[id]
	if !exists {
		return ErrNotFound
	}
	u.PasswordHash = passwordHash
	u.Salt = salt
	u.MustChangePassword = mustChangePassword
	u.UpdatedAt = time.Now().UTC()
	return nil
}

func (m *MemoryStore) UpdateUserLoginAttempt(ctx context.Context, email string, failedAttempts int, lockedUntil *time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	u, exists := m.usersByEm[email]
	if !exists {
		return ErrNotFound
	}
	u.FailedLoginAttempts = failedAttempts
	u.LockedUntil = lockedUntil
	u.UpdatedAt = time.Now().UTC()
	return nil
}

func (m *MemoryStore) ListUsers(ctx context.Context) ([]*User, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	list := make([]*User, 0, len(m.usersByID))
	for _, u := range m.usersByID {
		cp := *u
		list = append(list, &cp)
	}
	return list, nil
}

func (m *MemoryStore) CountUsers(ctx context.Context) (int, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.usersByID), nil
}

func (m *MemoryStore) CreateSession(ctx context.Context, session *Session) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := *session
	if cp.CreatedAt.IsZero() {
		cp.CreatedAt = time.Now().UTC()
	}
	m.sessions[session.ID] = &cp
	return nil
}

func (m *MemoryStore) GetSession(ctx context.Context, token string) (*Session, *User, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s, exists := m.sessions[token]
	if !exists {
		return nil, nil, ErrNotFound
	}
	if s.RevokedAt != nil {
		return nil, nil, errors.New("session has been revoked")
	}
	if time.Now().After(s.ExpiresAt) {
		return nil, nil, errors.New("session has expired")
	}
	u, exists := m.usersByID[s.UserID]
	if !exists {
		return nil, nil, errors.New("user for session not found")
	}
	cpSession := *s
	cpUser := *u
	return &cpSession, &cpUser, nil
}

func (m *MemoryStore) RevokeSession(ctx context.Context, token string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, exists := m.sessions[token]
	if !exists {
		return ErrNotFound
	}
	now := time.Now().UTC()
	s.RevokedAt = &now
	return nil
}

func (m *MemoryStore) RevokeUserSessions(ctx context.Context, userID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now().UTC()
	for _, s := range m.sessions {
		if s.UserID == userID && s.RevokedAt == nil {
			s.RevokedAt = &now
		}
	}
	return nil
}

func (m *MemoryStore) CreateAPIKey(ctx context.Context, key *APIKey) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := *key
	if cp.CreatedAt.IsZero() {
		cp.CreatedAt = time.Now().UTC()
	}
	m.apiKeys[key.KeyHash] = &cp
	m.apiKeysID[key.ID] = &cp
	return nil
}

func (m *MemoryStore) GetAPIKeyByHash(ctx context.Context, hash string) (*APIKey, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	k, exists := m.apiKeys[hash]
	if !exists {
		return nil, ErrNotFound
	}
	if k.RevokedAt != nil {
		return nil, errors.New("API key has been revoked")
	}
	if k.ExpiresAt != nil && time.Now().After(*k.ExpiresAt) {
		return nil, errors.New("API key has expired")
	}
	cp := *k
	return &cp, nil
}

func (m *MemoryStore) ListAPIKeys(ctx context.Context) ([]*APIKey, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	list := make([]*APIKey, 0, len(m.apiKeysID))
	for _, k := range m.apiKeysID {
		cp := *k
		list = append(list, &cp)
	}
	return list, nil
}

func (m *MemoryStore) RevokeAPIKey(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	k, exists := m.apiKeysID[id]
	if !exists {
		return ErrNotFound
	}
	now := time.Now().UTC()
	k.RevokedAt = &now
	return nil
}

func (m *MemoryStore) AppendAuditLog(ctx context.Context, entry *AuditLogEntry) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.auditSeq++
	cp := *entry
	cp.ID = m.auditSeq
	if cp.Timestamp.IsZero() {
		cp.Timestamp = time.Now().UTC()
	}
	m.auditLogs = append(m.auditLogs, &cp)
	return nil
}

func (m *MemoryStore) ListAuditLogs(ctx context.Context, limit int) ([]*AuditLogEntry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if limit <= 0 || limit > len(m.auditLogs) {
		limit = len(m.auditLogs)
	}
	res := make([]*AuditLogEntry, 0, limit)
	// Return in reverse chronological order
	for i := len(m.auditLogs) - 1; i >= 0 && len(res) < limit; i-- {
		cp := *m.auditLogs[i]
		res = append(res, &cp)
	}
	return res, nil
}

func (m *MemoryStore) CreateRecoveryExecution(ctx context.Context, exec *RecoveryExecution) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := *exec
	if cp.StartedAt.IsZero() {
		cp.StartedAt = time.Now().UTC()
	}
	m.recoveryByID[exec.ID] = &cp
	if exec.IdempotencyKey != "" {
		m.recoveryByKey[exec.IdempotencyKey] = &cp
	}
	m.recoveryList = append(m.recoveryList, &cp)
	return nil
}

func (m *MemoryStore) UpdateRecoveryExecution(ctx context.Context, exec *RecoveryExecution) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	existing, ok := m.recoveryByID[exec.ID]
	if !ok {
		return ErrNotFound
	}
	if exec.Status != "" {
		existing.Status = exec.Status
	}
	existing.DurationMs = exec.DurationMs
	existing.DurationStr = exec.DurationStr
	if exec.MetricsAfter != nil {
		existing.MetricsAfter = exec.MetricsAfter
	}
	existing.RiskAfter = exec.RiskAfter
	if exec.EscalatedAction != "" {
		existing.EscalatedAction = exec.EscalatedAction
	}
	if exec.ErrorMessage != "" {
		existing.ErrorMessage = exec.ErrorMessage
	}
	if exec.CompletedAt != nil {
		existing.CompletedAt = exec.CompletedAt
	}
	if exec.IdempotencyKey != "" {
		existing.IdempotencyKey = exec.IdempotencyKey
		m.recoveryByKey[exec.IdempotencyKey] = existing
	}
	return nil
}

func (m *MemoryStore) GetRecoveryExecution(ctx context.Context, id string) (*RecoveryExecution, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	exec, ok := m.recoveryByID[id]
	if !ok {
		return nil, ErrNotFound
	}
	cp := *exec
	return &cp, nil
}

func (m *MemoryStore) GetRecoveryExecutionByIdempotencyKey(ctx context.Context, key string) (*RecoveryExecution, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if key == "" {
		return nil, ErrNotFound
	}
	exec, ok := m.recoveryByKey[key]
	if !ok {
		return nil, ErrNotFound
	}
	cp := *exec
	return &cp, nil
}

func (m *MemoryStore) ListRecoveryExecutions(ctx context.Context, limit int) ([]*RecoveryExecution, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if limit <= 0 || limit > len(m.recoveryList) {
		limit = len(m.recoveryList)
	}
	res := make([]*RecoveryExecution, 0, limit)
	// Return in reverse chronological order
	for i := len(m.recoveryList) - 1; i >= 0 && len(res) < limit; i-- {
		cp := *m.recoveryList[i]
		res = append(res, &cp)
	}
	return res, nil
}

func (m *MemoryStore) UpsertService(ctx context.Context, svc *TopologyService) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := *svc
	cp.ResourceAttributes = cloneStringMap(svc.ResourceAttributes)
	if cp.CreatedAt.IsZero() {
		cp.CreatedAt = time.Now().UTC()
	}
	cp.UpdatedAt = time.Now().UTC()
	if cp.Namespace == "" {
		cp.Namespace = "default"
	}
	if cp.ClusterID == "" {
		cp.ClusterID = "cluster-primary"
	}
	if cp.Tier == "" {
		cp.Tier = "tier-2"
	}
	if cp.MonitoringMode == "" {
		cp.MonitoringMode = "observe"
	}
	if cp.TelemetrySource != nil {
		srcCopy := *cp.TelemetrySource
		if srcCopy.AuthToken != "" && !strings.HasPrefix(srcCopy.AuthToken, "enc::") {
			srcCopy.AuthToken, _ = EncryptCredential(srcCopy.AuthToken)
		}
		if srcCopy.AuthPassword != "" && !strings.HasPrefix(srcCopy.AuthPassword, "enc::") {
			srcCopy.AuthPassword, _ = EncryptCredential(srcCopy.AuthPassword)
		}
		cp.TelemetrySource = &srcCopy
	}
	if cp.Status == "" {
		cp.Status = cp.ComputeStatus()
	}
	m.servicesByID[cp.ID] = &cp
	return nil
}

func (m *MemoryStore) GetService(ctx context.Context, id string) (*TopologyService, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s, exists := m.servicesByID[id]
	if !exists {
		return nil, ErrNotFound
	}
	cp := *s
	cp.ResourceAttributes = cloneStringMap(s.ResourceAttributes)
	return &cp, nil
}

func (m *MemoryStore) DeleteService(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.servicesByID, id)
	// Cascade delete edges connected to this service
	for k, edge := range m.edgesByKey {
		if edge.SourceServiceID == id || edge.TargetServiceID == id {
			delete(m.edgesByKey, k)
			delete(m.edgesByID, edge.ID)
		}
	}
	return nil
}

func (m *MemoryStore) ListServices(ctx context.Context, namespace, clusterID string) ([]*TopologyService, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	res := make([]*TopologyService, 0, len(m.servicesByID))
	for _, s := range m.servicesByID {
		if namespace != "" && s.Namespace != namespace {
			continue
		}
		if clusterID != "" && s.ClusterID != clusterID {
			continue
		}
		cp := *s
		cp.ResourceAttributes = cloneStringMap(s.ResourceAttributes)
		res = append(res, &cp)
	}
	return res, nil
}

func cloneStringMap(input map[string]string) map[string]string {
	if input == nil { return nil }
	output := make(map[string]string, len(input))
	for key, value := range input { output[key] = value }
	return output
}

func (m *MemoryStore) edgeKey(sourceID, targetID, namespace, clusterID string) string {
	if namespace == "" {
		namespace = "default"
	}
	if clusterID == "" {
		clusterID = "cluster-primary"
	}
	return sourceID + "->" + targetID + "@" + namespace + ":" + clusterID
}

func (m *MemoryStore) UpsertEdge(ctx context.Context, edge *TopologyEdge) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := *edge
	if cp.Namespace == "" {
		cp.Namespace = "default"
	}
	if cp.ClusterID == "" {
		cp.ClusterID = "cluster-primary"
	}
	if cp.ID == "" {
		cp.ID = fmt.Sprintf("edge-%s-%s", cp.SourceServiceID, cp.TargetServiceID)
	}
	if cp.Source == "" {
		cp.Source = "manual"
	}
	if cp.State == "" {
		cp.State = "active"
	}
	if cp.EdgeType == "" {
		cp.EdgeType = "sync"
	}
	if cp.CreatedBy == "" {
		cp.CreatedBy = "system"
	}
	if cp.Source == "manual" {
		cp.IsManualOverride = true
		cp.Confidence = 1.0
	}
	if cp.CreatedAt.IsZero() {
		cp.CreatedAt = time.Now().UTC()
	}
	cp.UpdatedAt = time.Now().UTC()
	k := m.edgeKey(cp.SourceServiceID, cp.TargetServiceID, cp.Namespace, cp.ClusterID)
	m.edgesByKey[k] = &cp
	m.edgesByID[cp.ID] = &cp
	return nil
}

func (m *MemoryStore) GetEdgeByID(ctx context.Context, id string) (*TopologyEdge, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	e, exists := m.edgesByID[id]
	if !exists {
		// Fallback search through edgesByKey in case ID differed
		for _, edge := range m.edgesByKey {
			if edge.ID == id {
				cp := *edge
				return &cp, nil
			}
		}
		return nil, ErrNotFound
	}
	cp := *e
	return &cp, nil
}

func (m *MemoryStore) DeleteEdge(ctx context.Context, sourceID, targetID, namespace, clusterID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	k := m.edgeKey(sourceID, targetID, namespace, clusterID)
	if existing, ok := m.edgesByKey[k]; ok {
		delete(m.edgesByID, existing.ID)
	}
	delete(m.edgesByKey, k)
	return nil
}

func (m *MemoryStore) ListEdges(ctx context.Context, namespace, clusterID string) ([]*TopologyEdge, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	res := make([]*TopologyEdge, 0, len(m.edgesByKey))
	for _, e := range m.edgesByKey {
		if namespace != "" && e.Namespace != namespace {
			continue
		}
		if clusterID != "" && e.ClusterID != clusterID {
			continue
		}
		cp := *e
		res = append(res, &cp)
	}
	return res, nil
}

func (m *MemoryStore) ListSuggestions(ctx context.Context, namespace, clusterID string) ([]*TopologyEdge, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	res := make([]*TopologyEdge, 0)
	for _, e := range m.edgesByKey {
		if namespace != "" && e.Namespace != namespace {
			continue
		}
		if clusterID != "" && e.ClusterID != clusterID {
			continue
		}
		if e.State == "suggested" {
			cp := *e
			res = append(res, &cp)
		}
	}
	return res, nil
}

func (m *MemoryStore) AutoDiscoverSpanEdge(ctx context.Context, sourceID, targetID, namespace, clusterID string, latencyMs float64, isError bool) (*TopologyEdge, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if namespace == "" {
		namespace = "default"
	}
	if clusterID == "" {
		clusterID = "cluster-primary"
	}

	// Auto-provision services if they don't exist in registry
	if _, exists := m.servicesByID[sourceID]; !exists {
		now := time.Now().UTC()
		m.servicesByID[sourceID] = &TopologyService{
			ID:             sourceID,
			Name:           sourceID,
			Type:           "service",
			Namespace:      namespace,
			ClusterID:      clusterID,
			Tier:           "tier-2",
			Status:         "unconnected",
			MonitoringMode: "observe",
			CreatedAt:      now,
			UpdatedAt:      now,
		}
	}
	if _, exists := m.servicesByID[targetID]; !exists {
		now := time.Now().UTC()
		m.servicesByID[targetID] = &TopologyService{
			ID:             targetID,
			Name:           targetID,
			Type:           "service",
			Namespace:      namespace,
			ClusterID:      clusterID,
			Tier:           "tier-2",
			Status:         "unconnected",
			MonitoringMode: "observe",
			CreatedAt:      now,
			UpdatedAt:      now,
		}
	}

	k := m.edgeKey(sourceID, targetID, namespace, clusterID)
	edge, exists := m.edgesByKey[k]

	now := time.Now().UTC()
	errRateVal := 0.0
	if isError {
		errRateVal = 1.0
	}

	if exists {
		// Principle: Rejected suggestions must not reappear unless the user resets them!
		if edge.State == "rejected" {
			cp := *edge
			return &cp, nil
		}

		// Update sample statistics
		edge.SampleCount++
		// Exponential moving average for latency & error rate
		alpha := 0.2
		edge.LatencyMs = (1-alpha)*edge.LatencyMs + alpha*latencyMs
		edge.ErrorRate = (1-alpha)*edge.ErrorRate + alpha*errRateVal
		edge.UpdatedAt = now

		// If manual override was specified by human operator, preserve override flag and maximum confidence
		if !edge.IsManualOverride && edge.Source != "manual" {
			// Increase confidence asymptotically toward 0.99
			edge.Confidence = math.Min(0.99, 0.70+float64(edge.SampleCount)*0.02)
		}
		cp := *edge
		return &cp, nil
	}

	// Create new auto-discovered edge as SUGGESTED
	edgeID := "edge-" + sourceID + "-" + targetID
	newEdge := &TopologyEdge{
		ID:               edgeID,
		SourceServiceID:  sourceID,
		TargetServiceID:  targetID,
		Relationship:     "calls",
		Namespace:        namespace,
		ClusterID:        clusterID,
		Confidence:       0.75,
		IsManualOverride: false,
		CallsPerSec:      1.0,
		LatencyMs:        latencyMs,
		ErrorRate:        errRateVal,
		SampleCount:      1,
		Source:           "suggested",
		State:            "suggested",
		EdgeType:         "sync",
		CreatedBy:        "discovery",
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	m.edgesByKey[k] = newEdge
	m.edgesByID[edgeID] = newEdge
	cp := *newEdge
	return &cp, nil
}

func (m *MemoryStore) CreateTopologyVersion(ctx context.Context, version *TopologyVersion) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := *version
	if cp.CreatedAt.IsZero() {
		cp.CreatedAt = time.Now().UTC()
	}
	m.versionList = append(m.versionList, &cp)
	return nil
}

func (m *MemoryStore) GetLatestTopologyVersion(ctx context.Context) (*TopologyVersion, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if len(m.versionList) == 0 {
		return nil, ErrNotFound
	}
	cp := *m.versionList[len(m.versionList)-1]
	return &cp, nil
}

func (m *MemoryStore) ListTopologyVersions(ctx context.Context, limit int) ([]*TopologyVersion, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if limit <= 0 || limit > len(m.versionList) {
		limit = len(m.versionList)
	}
	res := make([]*TopologyVersion, 0, limit)
	for i := len(m.versionList) - 1; i >= 0 && len(res) < limit; i-- {
		cp := *m.versionList[i]
		res = append(res, &cp)
	}
	return res, nil
}

func (m *MemoryStore) Close() error {
	return nil
}
