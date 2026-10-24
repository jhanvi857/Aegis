package routes

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/aegis/go-services/api-gateway/storage"
	predictpb "github.com/aegis/go-services/shared/pb/predict"
	recoverypb "github.com/aegis/go-services/shared/pb/recovery"
	"google.golang.org/grpc"
)

type mockSparseServer struct {
	predictpb.UnimplementedPredictionServiceServer
	recoverypb.UnimplementedRecoveryServiceServer
	receivedNodeCount int
	injectedRootCause string
}

func (m *mockSparseServer) Predict(ctx context.Context, req *predictpb.PredictRequest) (*predictpb.PredictResponse, error) {
	nodes := req.Graph.Snapshot.Nodes
	m.receivedNodeCount = len(nodes)

	// Identify anomalous node from snapshot or telemetry batch
	detectedAnom := "svc-000"
	for _, n := range nodes {
		if cpu, ok := n.Metrics["cpu"]; ok && cpu > 80.0 {
			detectedAnom = n.Id
			break
		}
	}
	m.injectedRootCause = detectedAnom

	resp := &predictpb.PredictResponse{
		TimestampUnixNano:    time.Now().UnixNano(),
		Engine:               "sparse_two_stage",
		ModelVersion:         "2.1.0-sparse",
		CalibratedConfidence: 0.985,
		FailurePredictions: []*predictpb.FailurePrediction{
			{
				NodeId:                     detectedAnom,
				FailureProbability:         0.942,
				Confidence:                 0.985,
				EstimatedTimeToFailureSec: 30,
			},
		},
		RootCauses: []*predictpb.RootCause{
			{
				NodeId:              detectedAnom,
				Probability:         0.985,
				Explanation:         fmt.Sprintf("Sparse TGNN 2-hop neighborhood isolated anomaly on %s", detectedAnom),
				FaultFamily:         "cpu_stress",
				ContributingMetrics: []string{"cpu", "latency"},
			},
		},
		PropagationPaths: []*predictpb.PropagationPath{
			{
				PathNodes:       []string{detectedAnom, "svc-043"},
				PropagationRisk: 0.82,
			},
		},
		AffectedNodes: []*predictpb.AffectedNode{
			{
				NodeId:      detectedAnom,
				HopDistance: 0,
				EtaSeconds:  0,
				RiskScore:   0.985,
			},
			{
				NodeId:      "svc-043",
				HopDistance: 1,
				EtaSeconds:  2,
				RiskScore:   0.82,
			},
		},
		RecommendedActions: []*predictpb.ActionRecommendation{
			{
				ActionId:         fmt.Sprintf("act-scale-%s", detectedAnom),
				ActionType:       "SCALE_UP",
				TargetNodeId:     detectedAnom,
				Title:            fmt.Sprintf("Scale Up on %s", detectedAnom),
				Description:      "Mitigate CPU saturation via horizontal replica scale",
				RiskLevel:        "LOW",
				RequiresApproval: false,
				Confidence:       0.985,
			},
		},
	}
	return resp, nil
}

func TestSparseTGNN_200NodeTopology_ServingIntegration(t *testing.T) {
	// 1. Setup mock gRPC server
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to listen on tcp: %v", err)
	}
	defer listener.Close()

	grpcSrv := grpc.NewServer()
	mockSrv := &mockSparseServer{}
	predictpb.RegisterPredictionServiceServer(grpcSrv, mockSrv)
	recoverypb.RegisterRecoveryServiceServer(grpcSrv, mockSrv)

	go func() {
		_ = grpcSrv.Serve(listener)
	}()
	defer grpcSrv.Stop()

	// Connect GlobalGRPCClient to test gRPC server
	origClient := GlobalGRPCClient
	InitGRPCClient(listener.Addr().String())
	defer func() {
		GlobalGRPCClient = origClient
	}()

	store := storage.NewMemoryStore()
	SetAuthStore(store)
	router := SetupRouter(store)
	os.Setenv("AEGIS_SPARSE_THRESHOLD", "20")

	// Set up authenticated admin session
	ctx := context.Background()
	adminUser := &storage.User{
		ID:        "usr-admin-sparse",
		Name:      "Admin Alice",
		Email:     "admin@aegis.local",
		Role:      storage.RoleAdmin,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	_ = store.CreateUser(ctx, adminUser)
	adminToken := "tok-sparse-admin"
	_ = store.CreateSession(ctx, &storage.Session{
		ID:        adminToken,
		UserID:    adminUser.ID,
		ExpiresAt: time.Now().Add(2 * time.Hour),
		CreatedAt: time.Now().UTC(),
	})

	// 2. Generate 200-node topology payload
	const totalServices = 200
	services := make([]storage.TopologyService, totalServices)
	edges := make([]storage.TopologyEdge, 0, totalServices*2)

	for i := 0; i < totalServices; i++ {
		sid := fmt.Sprintf("svc-%03d", i)
		services[i] = storage.TopologyService{
			ID:          sid,
			Name:        fmt.Sprintf("Service %03d", i),
			Type:        "service",
			Tier:        "tier-2",
			Status:      "healthy",
			Replicas:    2,
			MaxReplicas: 10,
			CPU:         25.0,
			Memory:      30.0,
			Latency:     20.0,
			ErrorRate:   0.001,
		}
		if i > 0 {
			edges = append(edges, storage.TopologyEdge{
				SourceServiceID: fmt.Sprintf("svc-%03d", i-1),
				TargetServiceID: sid,
				Relationship:    "calls",
			})
		}
	}

	importBody, _ := json.Marshal(map[string]interface{}{
		"namespace":   "production-mesh",
		"clusterId":   "cluster-mesh-200",
		"description": "200-node benchmark serving mesh",
		"services":    services,
		"edges":       edges,
	})

	importReq := httptest.NewRequest("POST", "/api/registry/import", bytes.NewBuffer(importBody))
	importReq.Header.Set("Content-Type", "application/json")
	importReq.Header.Set("Authorization", "Bearer "+adminToken)
	importW := httptest.NewRecorder()
	router.ServeHTTP(importW, importReq)

	if importW.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK from /api/registry/import, got %d: %s", importW.Code, importW.Body.String())
	}

	// 3. Inject anomalous telemetry on node svc-042
	injectedNode := "svc-042"
	State.mu.Lock()
	if s, ok := State.Services[injectedNode]; ok {
		s.CPU = 96.5
		s.Latency = 650.0
		s.Status = "critical"
		s.ErrorRate = 0.12
	}
	State.mu.Unlock()

	// 4. Query /api/predict
	predictReq := httptest.NewRequest("GET", "/api/predict", nil)
	predictReq.Header.Set("Authorization", "Bearer "+adminToken)
	predictW := httptest.NewRecorder()
	router.ServeHTTP(predictW, predictReq)

	if predictW.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK from /api/predict, got %d: %s", predictW.Code, predictW.Body.String())
	}

	var predResp PredictionOutput
	if err := json.Unmarshal(predictW.Body.Bytes(), &predResp); err != nil {
		t.Fatalf("Failed to decode prediction response: %v", err)
	}

	// Verify all 200 nodes were passed to gRPC serving path
	if mockSrv.receivedNodeCount != 200 {
		t.Fatalf("Expected gRPC server to receive 200 nodes, got %d", mockSrv.receivedNodeCount)
	}

	// Assert response engine is sparse_two_stage
	if predResp.Engine != "sparse_two_stage" {
		t.Fatalf("Expected engine 'sparse_two_stage', got '%s'", predResp.Engine)
	}
	if predResp.ModelVersion != "2.1.0-sparse" {
		t.Fatalf("Expected modelVersion '2.1.0-sparse', got '%s'", predResp.ModelVersion)
	}
	if predResp.CalibratedConfidence <= 0.0 {
		t.Fatalf("Expected calibratedConfidence > 0, got %f", predResp.CalibratedConfidence)
	}

	// Assert root cause is among the injected node's neighborhood
	if predResp.RootCauseServiceID != injectedNode {
		t.Fatalf("Expected root cause to be '%s', got '%s'", injectedNode, predResp.RootCauseServiceID)
	}

	// 5. Query /readyz and verify it reports the sparse_two_stage engine
	readyReq := httptest.NewRequest("GET", "/readyz", nil)
	readyW := httptest.NewRecorder()
	router.ServeHTTP(readyW, readyReq)

	if readyW.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK from /readyz, got %d: %s", readyW.Code, readyW.Body.String())
	}

	var readyResp ReadinessResponse
	if err := json.Unmarshal(readyW.Body.Bytes(), &readyResp); err != nil {
		t.Fatalf("Failed to decode /readyz response: %v", err)
	}

	if readyResp.Engine != "sparse_two_stage" {
		t.Fatalf("Expected /readyz engine to report 'sparse_two_stage', got '%s'", readyResp.Engine)
	}
	if readyResp.Components["ml_inference"].Engine != "sparse_two_stage" {
		t.Fatalf("Expected /readyz ml_inference component engine 'sparse_two_stage', got '%s'", readyResp.Components["ml_inference"].Engine)
	}
}
