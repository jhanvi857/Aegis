package routes

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/aegis/go-services/api-gateway/storage"
	predictpb "github.com/aegis/go-services/shared/pb/predict"
	"github.com/gorilla/mux"
	"google.golang.org/grpc"
)

func appRequest(t *testing.T, method, path string, body interface{}) *http.Request {
	t.Helper()
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(method, path, bytes.NewReader(raw))
	return req.WithContext(context.WithValue(req.Context(), CallerCtxKey, &storage.CallerContext{
		ActorID: "user-1", Email: "owner@example.com", Role: storage.RoleAdmin,
	}))
}

func TestApplicationRegistrationDefaultsAndSingleUseKey(t *testing.T) {
	store := storage.NewMemoryStore()
	SetAuthStore(store)
	req := appRequest(t, http.MethodPost, "/api/apps", map[string]interface{}{
		"name":     "ShopKart",
		"services": []map[string]interface{}{{"id": "checkout", "name": "Checkout"}},
	})
	rr := httptest.NewRecorder()
	CreateApplicationHandler(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("registration status=%d body=%s", rr.Code, rr.Body.String())
	}
	var response struct {
		Application storage.Application `json:"application"`
		Key         string              `json:"api_key"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Key == "" || response.Application.Services[0].Mode != "recommend" || len(response.Application.Services[0].AllowedActions) != 0 {
		t.Fatalf("unexpected registration response: %#v", response)
	}
	keys, _ := store.ListAPIKeys(context.Background())
	if len(keys) != 1 || keys[0].KeyHash == response.Key {
		t.Fatal("application key was not stored as a hash")
	}
}

func TestApplicationTelemetryRejectsUnknownService(t *testing.T) {
	store := storage.NewMemoryStore()
	SetAuthStore(store)
	app := &storage.Application{ID: "app-test", Name: "Test", Services: []storage.ApplicationService{{ID: "known", Name: "Known"}}}
	_ = store.CreateApplication(context.Background(), app)
	sum := sha256Hex("agent-key")
	_ = store.CreateAPIKey(context.Background(), &storage.APIKey{ID: "key", Name: "test", KeyHash: sum, Scopes: "ingest", Role: storage.RoleOperator, AppID: app.ID})
	req := httptest.NewRequest(http.MethodPost, "/ingest", bytes.NewBufferString(`{"app_id":"app-test","service_id":"missing","metrics":{"cpu":1}}`))
	req = req.WithContext(context.WithValue(req.Context(), CallerCtxKey, &storage.CallerContext{IsAPIKey: true, APIKeyObj: &storage.APIKey{ID: "key", AppID: app.ID, Scopes: "ingest"}}))
	rr := httptest.NewRecorder()
	IngestApplicationTelemetryHandler(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("unknown service status=%d", rr.Code)
	}
}

func TestAgentResultRequiresDispatchedSignedAction(t *testing.T) {
	store := storage.NewMemoryStore()
	SetAuthStore(store)
	action := &storage.QueuedAction{
		ID: "act-test", AppID: "app-test", ServiceID: "checkout", Action: "FLUSH_CACHE",
		RiskLevel: "LOW", State: "DISPATCHED", SignedToken: "signed-token",
		ExpiresAt: time.Now().UTC().Add(time.Minute),
	}
	if err := store.CreateQueuedAction(context.Background(), action); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/agent/result", bytes.NewBufferString(`{"action_id":"act-test","status":"SUCCEEDED"}`))
	req = req.WithContext(context.WithValue(req.Context(), CallerCtxKey, &storage.CallerContext{
		IsAPIKey:  true,
		Email:     "agent@example.com",
		APIKeyObj: &storage.APIKey{ID: "key", AppID: "app-test", Scopes: "agent:result"},
	}))
	rr := httptest.NewRecorder()
	AgentResultHandler(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("missing signature status=%d body=%s", rr.Code, rr.Body.String())
	}
}

type e2ePredictionServer struct {
	predictpb.UnimplementedPredictionServiceServer
}

func (e2ePredictionServer) Predict(context.Context, *predictpb.PredictRequest) (*predictpb.PredictResponse, error) {
	return &predictpb.PredictResponse{
		Engine: "fake-python-baseline", ModelVersion: "test",
		FailurePredictions: []*predictpb.FailurePrediction{{NodeId: "app-e2e/checkout", FailureProbability: 0.95, Confidence: 0.9, EstimatedTimeToFailureSec: 12}},
		RecommendedActions: []*predictpb.ActionRecommendation{{ActionId: "pred-flush", ActionType: "FLUSH_CACHE", TargetNodeId: "app-e2e/checkout", Title: "Flush cache", Description: "Clear stale cache", RiskLevel: "LOW"}},
	}, nil
}

func TestApplicationRecoveryEndToEnd(t *testing.T) {
	t.Setenv("USE_KAFKA", "false")
	State = newControlPlaneState()
	store := storage.NewMemoryStore()
	SetAuthStore(store)
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	grpcServer := grpc.NewServer()
	predictpb.RegisterPredictionServiceServer(grpcServer, e2ePredictionServer{})
	go grpcServer.Serve(lis)
	defer grpcServer.Stop()
	GlobalGRPCClient = InitGRPCClient(lis.Addr().String())

	create := appRequest(t, http.MethodPost, "/api/apps", map[string]interface{}{
		"name": "E2E", "services": []map[string]interface{}{{
			"id": "checkout", "name": "Checkout", "mode": "execute", "allowed_actions": []string{"FLUSH_CACHE"},
		}},
	})
	createRR := httptest.NewRecorder()
	CreateApplicationHandler(createRR, create)
	if createRR.Code != http.StatusCreated {
		t.Fatalf("register status=%d body=%s", createRR.Code, createRR.Body.String())
	}
	var created struct {
		Application storage.Application `json:"application"`
		Key         string              `json:"api_key"`
	}
	_ = json.Unmarshal(createRR.Body.Bytes(), &created)
	if created.Application.ID == "" || created.Key == "" {
		t.Fatal("registration did not return application and one-time key")
	}
	appKeyContext := func(scope string) context.Context {
		return context.WithValue(context.Background(), CallerCtxKey, &storage.CallerContext{
			IsAPIKey: true, Email: "agent@example.com",
			APIKeyObj: &storage.APIKey{ID: "e2e-key", AppID: created.Application.ID, Scopes: scope},
		})
	}

	ingest := httptest.NewRequest(http.MethodPost, "/ingest", bytes.NewBufferString(`{"service_id":"checkout","metrics":{"cpu":99,"error_rate":0.9}}`))
	ingest = ingest.WithContext(appKeyContext("ingest"))
	ingestRR := httptest.NewRecorder()
	IngestApplicationTelemetryHandler(ingestRR, ingest)
	if ingestRR.Code != http.StatusAccepted {
		t.Fatalf("ingest status=%d body=%s", ingestRR.Code, ingestRR.Body.String())
	}
	var ingestResponse map[string]interface{}
	_ = json.Unmarshal(ingestRR.Body.Bytes(), &ingestResponse)
	if ingestResponse["prediction"] == nil {
		t.Fatal("ingest response did not include Python prediction")
	}
	State.mu.RLock()
	storedPrediction, stored := State.LatestPredictions[created.Application.ID+"/checkout"]
	State.mu.RUnlock()
	if !stored || storedPrediction.Engine != "fake-python-baseline" {
		t.Fatalf("prediction was not stored per app/service: stored=%v engine=%q", stored, storedPrediction.Engine)
	}

	propose := appRequest(t, http.MethodPost, "/api/apps/"+created.Application.ID+"/actions", map[string]interface{}{
		"service_id": "checkout", "action": "FLUSH_CACHE", "evidence": map[string]interface{}{"error_rate": 0.9}, "expected_effect": "Clear stale cache",
	})
	propose = mux.SetURLVars(propose, map[string]string{"id": created.Application.ID})
	propose = propose.WithContext(context.WithValue(propose.Context(), CallerCtxKey, &storage.CallerContext{ActorID: "owner", Email: "owner@example.com", Role: storage.RoleAdmin}))
	proposeRR := httptest.NewRecorder()
	ProposeApplicationActionHandler(proposeRR, propose)
	if proposeRR.Code != http.StatusOK {
		t.Fatalf("propose status=%d body=%s", proposeRR.Code, proposeRR.Body.String())
	}
	var action storage.QueuedAction
	_ = json.Unmarshal(proposeRR.Body.Bytes(), &action)
	if action.State != "PROPOSED" {
		t.Fatalf("expected PROPOSED, got %s", action.State)
	}

	approve := appRequest(t, http.MethodPost, "/api/apps/"+created.Application.ID+"/actions/"+action.ID+"/approve", nil)
	approve = mux.SetURLVars(approve, map[string]string{"id": created.Application.ID, "action": action.ID})
	approve = approve.WithContext(context.WithValue(approve.Context(), CallerCtxKey, &storage.CallerContext{ActorID: "owner", Email: "owner@example.com", Role: storage.RoleAdmin}))
	approveRR := httptest.NewRecorder()
	ApproveApplicationActionHandler(approveRR, approve)
	if approveRR.Code != http.StatusOK {
		t.Fatalf("approve status=%d body=%s", approveRR.Code, approveRR.Body.String())
	}

	poll := httptest.NewRequest(http.MethodGet, "/agent/poll", nil).WithContext(appKeyContext("agent:poll"))
	pollRR := httptest.NewRecorder()
	AgentPollHandler(pollRR, poll)
	if pollRR.Code != http.StatusOK || !strings.Contains(pollRR.Body.String(), `"state":"DISPATCHED"`) {
		t.Fatalf("poll status=%d body=%s", pollRR.Code, pollRR.Body.String())
	}
	unsigned := httptest.NewRequest(http.MethodPost, "/agent/result", bytes.NewBufferString(`{"action_id":"`+action.ID+`","status":"SUCCEEDED"}`)).WithContext(appKeyContext("agent:result"))
	unsignedRR := httptest.NewRecorder()
	AgentResultHandler(unsignedRR, unsigned)
	if unsignedRR.Code != http.StatusForbidden {
		t.Fatalf("unsigned result status=%d", unsignedRR.Code)
	}
	signed := httptest.NewRequest(http.MethodPost, "/agent/result", bytes.NewBufferString(`{"action_id":"`+action.ID+`","status":"SUCCEEDED","signed_token":"`+action.SignedToken+`"}`)).WithContext(appKeyContext("agent:result"))
	signedRR := httptest.NewRecorder()
	AgentResultHandler(signedRR, signed)
	if signedRR.Code != http.StatusOK {
		t.Fatalf("signed result status=%d body=%s", signedRR.Code, signedRR.Body.String())
	}

	outside := appRequest(t, http.MethodPost, "/api/apps/"+created.Application.ID+"/actions", map[string]interface{}{"service_id": "checkout", "action": "RESTART"})
	outside = mux.SetURLVars(outside, map[string]string{"id": created.Application.ID})
	outside = outside.WithContext(context.WithValue(outside.Context(), CallerCtxKey, &storage.CallerContext{ActorID: "owner", Email: "owner@example.com", Role: storage.RoleAdmin}))
	outsideRR := httptest.NewRecorder()
	ProposeApplicationActionHandler(outsideRR, outside)
	if outsideRR.Code != http.StatusForbidden {
		t.Fatalf("outside allowlist status=%d", outsideRR.Code)
	}

	observeCreate := appRequest(t, http.MethodPost, "/api/apps", map[string]interface{}{
		"name": "Observe", "services": []map[string]interface{}{{"id": "api", "name": "API", "mode": "observe", "allowed_actions": []string{"FLUSH_CACHE"}}},
	})
	observeRR := httptest.NewRecorder()
	CreateApplicationHandler(observeRR, observeCreate)
	var observed struct {
		Application storage.Application `json:"application"`
	}
	_ = json.Unmarshal(observeRR.Body.Bytes(), &observed)
	observeAction := appRequest(t, http.MethodPost, "/api/apps/"+observed.Application.ID+"/actions", map[string]interface{}{"service_id": "api", "action": "FLUSH_CACHE"})
	observeAction = mux.SetURLVars(observeAction, map[string]string{"id": observed.Application.ID})
	observeAction = observeAction.WithContext(context.WithValue(observeAction.Context(), CallerCtxKey, &storage.CallerContext{ActorID: "owner", Email: "owner@example.com", Role: storage.RoleAdmin}))
	observeActionRR := httptest.NewRecorder()
	ProposeApplicationActionHandler(observeActionRR, observeAction)
	var observedQueued storage.QueuedAction
	_ = json.Unmarshal(observeActionRR.Body.Bytes(), &observedQueued)
	observeApprove := appRequest(t, http.MethodPost, "/api/apps/"+observed.Application.ID+"/actions/"+observedQueued.ID+"/approve", nil)
	observeApprove = mux.SetURLVars(observeApprove, map[string]string{"id": observed.Application.ID, "action": observedQueued.ID})
	observeApprove = observeApprove.WithContext(context.WithValue(observeApprove.Context(), CallerCtxKey, &storage.CallerContext{ActorID: "owner", Email: "owner@example.com", Role: storage.RoleAdmin}))
	observeApproveRR := httptest.NewRecorder()
	ApproveApplicationActionHandler(observeApproveRR, observeApprove)
	if observeApproveRR.Code != http.StatusForbidden {
		t.Fatalf("observe approval status=%d", observeApproveRR.Code)
	}
}
func sha256Hex(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
