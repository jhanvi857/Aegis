package storage

import (
	"context"
	"testing"
	"time"
)

func TestArgon2id_HashAndVerify(t *testing.T) {
	password := "CorrectHorseBatteryStaple#2026"
	salt, err := GenerateSalt()
	if err != nil {
		t.Fatalf("failed to generate salt: %v", err)
	}

	hash, err := HashPassword(password, salt)
	if err != nil {
		t.Fatalf("failed to hash password: %v", err)
	}

	if !VerifyPassword(password, salt, hash) {
		t.Errorf("expected password verification to succeed")
	}

	if VerifyPassword("WrongPassword", salt, hash) {
		t.Errorf("expected wrong password verification to fail")
	}
}

func TestMemoryStore_Lifecycle(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()

	// 1. User creation and lookup
	u := &User{
		ID:           "usr-1",
		Name:         "Alice Admin",
		Email:        "alice@enterprise.io",
		PasswordHash: "hash123",
		Salt:         "salt123",
		Role:         RoleAdmin,
	}

	if err := store.CreateUser(ctx, u); err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	// Conflict on duplicate email
	if err := store.CreateUser(ctx, u); err != ErrConflict {
		t.Errorf("expected ErrConflict on duplicate user, got %v", err)
	}

	retrieved, err := store.GetUserByEmail(ctx, "alice@enterprise.io")
	if err != nil || retrieved.ID != "usr-1" {
		t.Fatalf("failed to retrieve user by email: %v", err)
	}

	// 2. Count and list users
	count, err := store.CountUsers(ctx)
	if err != nil || count != 1 {
		t.Errorf("expected 1 user, got %d (err: %v)", count, err)
	}

	// 3. Update role
	if err := store.UpdateUserRole(ctx, "usr-1", RoleOperator); err != nil {
		t.Fatalf("failed to update user role: %v", err)
	}
	retrieved2, _ := store.GetUserByID(ctx, "usr-1")
	if retrieved2.Role != RoleOperator {
		t.Errorf("expected role operator, got %s", retrieved2.Role)
	}

	// 4. Session management
	sess := &Session{
		ID:        "token-abc",
		UserID:    "usr-1",
		ExpiresAt: time.Now().Add(1 * time.Hour),
	}
	if err := store.CreateSession(ctx, sess); err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	s, userSess, err := store.GetSession(ctx, "token-abc")
	if err != nil || s.ID != "token-abc" || userSess.ID != "usr-1" {
		t.Fatalf("failed to get active session: %v", err)
	}

	// Revoke session
	if err := store.RevokeSession(ctx, "token-abc"); err != nil {
		t.Fatalf("failed to revoke session: %v", err)
	}
	_, _, err = store.GetSession(ctx, "token-abc")
	if err == nil {
		t.Errorf("expected error getting revoked session")
	}

	// 5. Machine API Key
	apiKey := &APIKey{
		ID:        "key-1",
		Name:      "telemetry-agent",
		KeyPrefix: "aegis_ak_test",
		KeyHash:   "hash_key_1",
		Scopes:    "telemetry:write,chaos:inject",
		Role:      RoleOperator,
		CreatedBy: "usr-1",
	}
	if err := store.CreateAPIKey(ctx, apiKey); err != nil {
		t.Fatalf("failed to create API key: %v", err)
	}
	retKey, err := store.GetAPIKeyByHash(ctx, "hash_key_1")
	if err != nil || retKey.ID != "key-1" {
		t.Fatalf("failed to get API key: %v", err)
	}
	if !retKey.HasScope("telemetry:write") {
		t.Errorf("expected scope telemetry:write")
	}
	if retKey.HasScope("admin") {
		t.Errorf("did not expect admin scope")
	}

	// 6. Audit log
	entry := &AuditLogEntry{
		ActorID:      "usr-1",
		ActorEmail:   "alice@enterprise.io",
		ActorRole:    RoleOperator,
		Action:       "chaos.inject",
		ResultStatus: "SUCCESS",
	}
	if err := store.AppendAuditLog(ctx, entry); err != nil {
		t.Fatalf("failed to append audit log: %v", err)
	}
	logs, err := store.ListAuditLogs(ctx, 10)
	if err != nil || len(logs) != 1 || logs[0].Action != "chaos.inject" {
		t.Fatalf("failed to list audit logs: %v", err)
	}
}

func TestMemoryStore_Phase3_TopologyAndAutoDiscovery(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()

	// 1. Service registration & CRUD
	svc := &TopologyService{
		ID:           "payment-service",
		Name:         "Payment Processing",
		Type:         "service",
		Namespace:    "finance",
		ClusterID:    "us-east-k8s",
		Tier:         "tier-1-ingress",
		Capabilities: "scale,restart,reroute",
		Owner:        "payments-team",
		Version:      "v2.4.1",
		Replicas:     3,
		MaxReplicas:  15,
		Status:       "healthy",
	}

	if err := store.UpsertService(ctx, svc); err != nil {
		t.Fatalf("failed to upsert service: %v", err)
	}

	retSvc, err := store.GetService(ctx, "payment-service")
	if err != nil || retSvc.Name != "Payment Processing" || retSvc.Tier != "tier-1-ingress" {
		t.Fatalf("failed to get service: %v, retrieved: %+v", err, retSvc)
	}

	// Filtered listing
	list, err := store.ListServices(ctx, "finance", "us-east-k8s")
	if err != nil || len(list) != 1 {
		t.Errorf("expected 1 service in finance namespace, got %d", len(list))
	}
	emptyList, err := store.ListServices(ctx, "marketing", "")
	if err != nil || len(emptyList) != 0 {
		t.Errorf("expected 0 services in marketing, got %d", len(emptyList))
	}

	// 2. Manual Edge creation
	edge := &TopologyEdge{
		ID:               "edge-pay-db",
		SourceServiceID:  "payment-service",
		TargetServiceID:  "payment-db",
		Relationship:     "calls",
		Namespace:        "finance",
		ClusterID:        "us-east-k8s",
		Confidence:       1.0,
		IsManualOverride: true,
	}
	if err := store.UpsertEdge(ctx, edge); err != nil {
		t.Fatalf("failed to upsert edge: %v", err)
	}

	edges, err := store.ListEdges(ctx, "finance", "us-east-k8s")
	if err != nil || len(edges) != 1 || edges[0].Confidence != 1.0 {
		t.Fatalf("expected 1 manual edge with 1.0 confidence, got %+v", edges)
	}

	// 3. Trace Auto-Discovery
	// Auto-discovering span between auth-service and cache-redis
	discovered, err := store.AutoDiscoverSpanEdge(ctx, "auth-service", "cache-redis", "identity", "us-east-k8s", 15.4, false)
	if err != nil {
		t.Fatalf("failed to auto-discover span edge: %v", err)
	}
	if discovered.Confidence < 0.70 || discovered.Confidence > 0.80 {
		t.Errorf("expected initial confidence around 0.75, got %.2f", discovered.Confidence)
	}
	if discovered.IsManualOverride {
		t.Errorf("auto-discovered edge must not have isManualOverride set to true")
	}

	// Auto-provisioned services should now exist in registry
	authSvc, err := store.GetService(ctx, "auth-service")
	if err != nil || authSvc == nil {
		t.Errorf("expected auth-service to be auto-provisioned in registry")
	}

	// Subsequent trace span increments samples and asymptotically increases confidence
	for i := 0; i < 5; i++ {
		_, _ = store.AutoDiscoverSpanEdge(ctx, "auth-service", "cache-redis", "identity", "us-east-k8s", 14.0, false)
	}
	updatedDiscovered, _ := store.AutoDiscoverSpanEdge(ctx, "auth-service", "cache-redis", "identity", "us-east-k8s", 12.0, false)
	if updatedDiscovered.SampleCount < 5 {
		t.Errorf("expected sampleCount >= 5, got %d", updatedDiscovered.SampleCount)
	}
	if updatedDiscovered.Confidence <= discovered.Confidence {
		t.Errorf("expected confidence to increase with samples: %.2f vs %.2f", updatedDiscovered.Confidence, discovered.Confidence)
	}

	// Manual override protection: auto-discovery on manual edge never downgrades override flag
	discManual, _ := store.AutoDiscoverSpanEdge(ctx, "payment-service", "payment-db", "finance", "us-east-k8s", 4.2, false)
	if !discManual.IsManualOverride || discManual.Confidence != 1.0 {
		t.Errorf("auto-discovery must preserve manual override and 1.0 confidence, got isManual=%v conf=%.2f",
			discManual.IsManualOverride, discManual.Confidence)
	}

	// 4. Topology Snapshots / Versioning
	ver := &TopologyVersion{
		ID:          "ver-1",
		Version:     1,
		Description: "Initial dynamic topology import",
		Snapshot: map[string]interface{}{
			"services": []string{"payment-service", "payment-db", "auth-service", "cache-redis"},
		},
		CreatedBy: "admin@enterprise.io",
	}
	if err := store.CreateTopologyVersion(ctx, ver); err != nil {
		t.Fatalf("failed to create topology version: %v", err)
	}

	latestVer, err := store.GetLatestTopologyVersion(ctx)
	if err != nil || latestVer.Version != 1 {
		t.Fatalf("failed to retrieve latest version: %v, got %+v", err, latestVer)
	}
}

