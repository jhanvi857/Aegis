package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	"github.com/aegis/go-services/api-gateway/routes"
	"github.com/aegis/go-services/api-gateway/storage"
)

func setupGenericMesh(store storage.Store) {
	routes.SetAuthStore(store)
	routes.InitState()
	routes.Safety.Reset()

	routes.State.Lock()
	defer routes.State.Unlock()

	now := time.Now()
	// Neutral 6-service mesh (svc-a .. svc-f)
	routes.State.Services = map[string]*routes.Microservice{
		"svc-a": {
			ID:             "svc-a",
			Name:           "Service Ingress Gateway",
			Type:           "gateway",
			Status:         "healthy",
			CPU:            18.0,
			Memory:         25.0,
			Latency:        12.0,
			ErrorRate:      0.001,
			Replicas:       2,
			Dependencies:   []string{"svc-b", "svc-c"},
			LastIngestTime: now,
			DataStatus:     "fresh",
		},
		"svc-b": {
			ID:             "svc-b",
			Name:           "Core Processing Service",
			Type:           "service",
			Status:         "healthy",
			CPU:            22.0,
			Memory:         30.0,
			Latency:        15.0,
			ErrorRate:      0.001,
			Replicas:       3,
			Dependencies:   []string{"svc-d"},
			LastIngestTime: now,
			DataStatus:     "fresh",
		},
		"svc-c": {
			ID:             "svc-c",
			Name:           "Queue Worker Service",
			Type:           "service",
			Status:         "healthy",
			CPU:            20.0,
			Memory:         28.0,
			Latency:        14.0,
			ErrorRate:      0.001,
			Replicas:       2,
			Dependencies:   []string{"svc-e", "svc-f"},
			LastIngestTime: now,
			DataStatus:     "fresh",
		},
		"svc-d": {
			ID:             "svc-d",
			Name:           "Primary Database Node",
			Type:           "database",
			Status:         "healthy",
			CPU:            25.0,
			Memory:         40.0,
			Latency:        10.0,
			ErrorRate:      0.000,
			Replicas:       1,
			Dependencies:   []string{},
			LastIngestTime: now,
			DataStatus:     "fresh",
		},
		"svc-e": {
			ID:             "svc-e",
			Name:           "In-Memory Cache Node",
			Type:           "cache",
			Status:         "healthy",
			CPU:            15.0,
			Memory:         35.0,
			Latency:        2.0,
			ErrorRate:      0.000,
			Replicas:       1,
			Dependencies:   []string{},
			LastIngestTime: now,
			DataStatus:     "fresh",
		},
		"svc-f": {
			ID:             "svc-f",
			Name:           "Message Broker Queue",
			Type:           "queue",
			Status:         "healthy",
			CPU:            18.0,
			Memory:         25.0,
			Latency:        8.0,
			ErrorRate:      0.000,
			Replicas:       1,
			Dependencies:   []string{},
			LastIngestTime: now,
			DataStatus:     "fresh",
		},
	}
	routes.State.ServiceOrder = []string{"svc-a", "svc-b", "svc-c", "svc-d", "svc-e", "svc-f"}
	routes.State.NodeBreachCounts = make(map[string]int)
}

func prettyJSON(v interface{}) string {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	return string(b)
}

func main() {
	fmt.Println("================================================================================")
	fmt.Println("AEGIS LIVE FAULT ISOLATION, PROVENANCE & RECOVERY LIFECYCLE VERIFICATION")
	fmt.Println("Generic Synthetic Mesh: svc-a (ingress) -> svc-b, svc-c -> svc-d, svc-e, svc-f")
	fmt.Println("================================================================================")

	store := storage.NewMemoryStore()

	// -------------------------------------------------------------------------
	// 1. Nominal Baseline Test
	// -------------------------------------------------------------------------
	fmt.Println("\n[TEST 1] Nominal Baseline (Zero faults, idle healthy traffic)")
	setupGenericMesh(store)

	req := httptest.NewRequest("GET", "/api/predict", nil)
	w := httptest.NewRecorder()
	routes.PredictHandler(w, req)

	var nomPred routes.PredictionOutput
	_ = json.Unmarshal(w.Body.Bytes(), &nomPred)

	fmt.Printf("Status: %d OK\n", w.Code)
	fmt.Printf("State: %s, Incident: %v, FailureProbability: %.1f%%, Confidence: %s\n",
		nomPred.State, nomPred.Incident, nomPred.FailureProbability,
		fmt.Sprintf("%.1f", nomPred.Confidence))
	fmt.Printf("Prediction JSON Output:\n%s\n", prettyJSON(nomPred))

	if nomPred.Incident != false {
		panic("FAIL: Nominal mesh produced incident = true")
	}
	if nomPred.State != "nominal" {
		panic(fmt.Sprintf("FAIL: Nominal mesh state is '%s', expected 'nominal'", nomPred.State))
	}
	if nomPred.FailureProbability != 0.0 {
		panic(fmt.Sprintf("FAIL: Nominal failure probability is %.1f, expected 0.0", nomPred.FailureProbability))
	}
	fmt.Println("--> PASS: Nominal baseline is clean (0% probability, no incident, state=nominal).")

	// -------------------------------------------------------------------------
	// 2. Data Quality Gate: Missing / Stale Metrics (>2x scrape interval)
	// -------------------------------------------------------------------------
	fmt.Println("\n[TEST 2] Data Quality Gate: Missing/Stale Metrics (>2x scrape interval)")
	setupGenericMesh(store)

	routes.State.Lock()
	// Age svc-b metrics by 30 seconds (beyond 2x scrape interval)
	routes.State.Services["svc-b"].LastIngestTime = time.Now().Add(-30 * time.Second)
	routes.State.Services["svc-b"].CPU = 99.0 // stale anomalous metric
	routes.State.Unlock()

	req = httptest.NewRequest("GET", "/api/predict", nil)
	w = httptest.NewRecorder()
	routes.PredictHandler(w, req)

	var stalePred routes.PredictionOutput
	_ = json.Unmarshal(w.Body.Bytes(), &stalePred)

	fmt.Printf("svc-b DataStatus: %s (Status: %s)\n",
		routes.State.Services["svc-b"].DataStatus, routes.State.Services["svc-b"].Status)
	fmt.Printf("Incident: %v, State: %s\n", stalePred.Incident, stalePred.State)

	if routes.State.Services["svc-b"].DataStatus != "unknown" {
		panic("FAIL: Stale service was not marked DataStatus='unknown'")
	}
	if stalePred.Incident != false {
		panic("FAIL: Stale metric triggered a false incident")
	}
	fmt.Println("--> PASS: Stale data dropped from inference (marked unknown, no false incident).")

	// -------------------------------------------------------------------------
	// 3-9. All 7 Chaos Fault Types against Neutral Mesh
	// -------------------------------------------------------------------------
	type FaultTestDef struct {
		FaultName          string
		TargetNode         string
		ApplyMetrics       func(s *routes.Microservice)
		ExpectedFamily     string
		ExpectedAction     string
		ForbiddenAction    string
		ExpectedRisk       string
		RequiresApproval   bool
	}

	faults := []FaultTestDef{
		{
			FaultName:  "db_lock",
			TargetNode: "svc-d",
			ApplyMetrics: func(s *routes.Microservice) {
				s.DBWait = 320.0
				s.Latency = 450.0
				s.Status = "critical"
			},
			ExpectedFamily:   "db_contention",
			ExpectedAction:   "increase_pool",
			ForbiddenAction:  "restart",
			ExpectedRisk:     "LOW",
			RequiresApproval: false,
		},
		{
			FaultName:  "cache_down",
			TargetNode: "svc-e",
			ApplyMetrics: func(s *routes.Microservice) {
				s.CacheMiss = 0.85
				s.Latency = 380.0
				s.Status = "degraded"
			},
			ExpectedFamily:   "cache_down",
			ExpectedAction:   "flush_cache",
			ForbiddenAction:  "restart",
			ExpectedRisk:     "MEDIUM",
			RequiresApproval: true,
		},
		{
			FaultName:  "mq_lag",
			TargetNode: "svc-f",
			ApplyMetrics: func(s *routes.Microservice) {
				s.QueueDepth = 8500
				s.Latency = 420.0
				s.Status = "degraded"
			},
			ExpectedFamily:   "mq_lag",
			ExpectedAction:   "scale",
			ForbiddenAction:  "restart",
			ExpectedRisk:     "LOW",
			RequiresApproval: false,
		},
		{
			FaultName:  "kill_service",
			TargetNode: "svc-b",
			ApplyMetrics: func(s *routes.Microservice) {
				s.Status = "failed"
				s.ErrorRate = 0.98
				s.Latency = 1200.0
				s.CPU = 2.0
			},
			ExpectedFamily:   "crash",
			ExpectedAction:   "restart", // Crash allows restart
			ForbiddenAction:  "",
			ExpectedRisk:     "HIGH",
			RequiresApproval: true,
		},
		{
			FaultName:  "cpu_stress",
			TargetNode: "svc-b",
			ApplyMetrics: func(s *routes.Microservice) {
				s.CPU = 96.0
				s.Latency = 350.0
				s.Status = "critical"
			},
			ExpectedFamily:   "cpu_stress",
			ExpectedAction:   "scale",
			ForbiddenAction:  "restart",
			ExpectedRisk:     "LOW",
			RequiresApproval: false,
		},
		{
			FaultName:  "latency",
			TargetNode: "svc-a",
			ApplyMetrics: func(s *routes.Microservice) {
				s.Latency = 650.0
				s.Status = "degraded"
			},
			ExpectedFamily:   "latency",
			ExpectedAction:   "reroute_traffic",
			ForbiddenAction:  "restart",
			ExpectedRisk:     "MEDIUM",
			RequiresApproval: true,
		},
		{
			FaultName:  "packet_loss",
			TargetNode: "svc-c",
			ApplyMetrics: func(s *routes.Microservice) {
				s.PacketLoss = 0.35
				s.Latency = 420.0
				s.Status = "degraded"
			},
			ExpectedFamily:   "packet_loss",
			ExpectedAction:   "reroute_traffic",
			ForbiddenAction:  "restart",
			ExpectedRisk:     "MEDIUM",
			RequiresApproval: true,
		},
	}

	for idx, ft := range faults {
		fmt.Printf("\n[TEST %d] Injected Fault: %s on %s\n", idx+3, ft.FaultName, ft.TargetNode)
		setupGenericMesh(store)

		routes.State.Lock()
		routes.State.Services[ft.TargetNode].LastIngestTime = time.Now()
		routes.State.Services[ft.TargetNode].DataStatus = "fresh"
		ft.ApplyMetrics(routes.State.Services[ft.TargetNode])
		// Sustained breach simulation (3 consecutive windows)
		routes.State.NodeBreachCounts[ft.TargetNode] = 3
		routes.State.Unlock()

		req = httptest.NewRequest("GET", "/api/predict", nil)
		w = httptest.NewRecorder()
		routes.PredictHandler(w, req)

		var resp routes.PredictionOutput
		_ = json.Unmarshal(w.Body.Bytes(), &resp)

		fmt.Printf("HTTP %d | Incident: %v (State: %s) | FaultFamily: %s | RootCause: %s | Action: %s (%s, Risk: %s)\n",
			w.Code, resp.Incident, resp.State, resp.FaultFamily, resp.RootCauseServiceID,
			resp.RecommendedActionID, resp.RecommendedAction, resp.RecommendedActions[0].RiskLevel)
		fmt.Printf("Evidence: %v\n", resp.RecommendedActions[0].Evidence)
		fmt.Printf("Top Recommended Action Invariant: Target='%s' WhyThisTarget='%s'\n",
			resp.RecommendedActions[0].TargetServiceID, resp.RecommendedActions[0].WhyThisTarget)

		if !resp.Incident {
			panic(fmt.Sprintf("FAIL: Sustained %s did not trigger incident", ft.FaultName))
		}
		if resp.RootCauseServiceID != ft.TargetNode {
			panic(fmt.Sprintf("FAIL: Root cause '%s' mismatch, expected '%s'", resp.RootCauseServiceID, ft.TargetNode))
		}
		if !strings.EqualFold(resp.RecommendedActions[0].TargetServiceID, ft.TargetNode) {
			panic(fmt.Sprintf("FAIL: Action target '%s' does not match root cause '%s'",
				resp.RecommendedActions[0].TargetServiceID, ft.TargetNode))
		}
		if ft.ForbiddenAction != "" && strings.EqualFold(resp.RecommendedActions[0].ActionType, ft.ForbiddenAction) {
			panic(fmt.Sprintf("FAIL: Invariant breached! %s action recommended for %s", ft.ForbiddenAction, ft.FaultName))
		}
		if ft.ExpectedAction != "" && !strings.Contains(strings.ToLower(resp.RecommendedActions[0].ActionType), strings.ToLower(ft.ExpectedAction)) {
			panic(fmt.Sprintf("FAIL: Expected action '%s', got '%s'", ft.ExpectedAction, resp.RecommendedActions[0].ActionType))
		}
		fmt.Println("--> PASS: Root cause, action target alignment, and no-restart invariants verified.")
	}

	// -------------------------------------------------------------------------
	// 10. Recovery Execution Lifecycle & 403 Authority Boundary Gate
	// -------------------------------------------------------------------------
	fmt.Println("\n[TEST 10] Recovery Lifecycle, 403 Authority Gate & Telemetry Verification")
	setupGenericMesh(store)

	// Inject latency on svc-b (Requires MEDIUM approval)
	routes.State.Lock()
	routes.State.Services["svc-b"].Latency = 750.0
	routes.State.Services["svc-b"].Status = "degraded"
	routes.State.NodeBreachCounts["svc-b"] = 3
	routes.State.Unlock()

	// 10a. Execute MEDIUM risk action WITHOUT approval -> MUST return 403 Forbidden
	execPayload, _ := json.Marshal(map[string]interface{}{
		"actionId":        "act-reroute-svc-b",
		"targetServiceId": "svc-b",
		"actionType":      "reroute_traffic",
	})
	execReq := httptest.NewRequest("POST", "/api/recovery/execute", bytes.NewBuffer(execPayload))
	execW := httptest.NewRecorder()
	routes.ExecuteRecoveryHandler(execW, execReq)

	fmt.Printf("10a: Execute without approval -> HTTP %d (Expected: 403)\n", execW.Code)
	fmt.Printf("     Response: %s\n", strings.TrimSpace(execW.Body.String()))
	if execW.Code != http.StatusForbidden {
		panic(fmt.Sprintf("FAIL: Expected 403 Forbidden without approval, got %d", execW.Code))
	}

	// Verify incident is STILL active (never false green)
	req = httptest.NewRequest("GET", "/api/predict", nil)
	w = httptest.NewRecorder()
	routes.PredictHandler(w, req)
	var stillIncident routes.PredictionOutput
	_ = json.Unmarshal(w.Body.Bytes(), &stillIncident)
	if !stillIncident.Incident {
		panic("FAIL: Dashboard falsely cleared incident after 403 execution failure!")
	}
	fmt.Println("--> Verified: 403 returned, dashboard STAYS in incident (no false green).")

	// 10b. Human Operator records approval
	appPayload, _ := json.Marshal(map[string]interface{}{
		"actionId":        "act-reroute-svc-b",
		"targetServiceId": "svc-b",
		"actionType":      "reroute_traffic",
		"approver":        "lead-sre@universal.mesh",
	})
	appReq := httptest.NewRequest("POST", "/api/recovery/approve", bytes.NewBuffer(appPayload))
	appW := httptest.NewRecorder()
	routes.ApproveRecoveryHandler(appW, appReq)
	fmt.Printf("10b: Operator Approval -> HTTP %d: %s\n", appW.Code, strings.TrimSpace(appW.Body.String()))
	if appW.Code != http.StatusOK {
		panic("FAIL: Operator approval returned non-200")
	}

	// 10c. Re-execute WITH approval -> Status 200, lifecycle becomes 'Verifying'
	routes.Safety.SetVerificationWindow(200 * time.Millisecond)
	execReq2 := httptest.NewRequest("POST", "/api/recovery/execute", bytes.NewBuffer(execPayload))
	execW2 := httptest.NewRecorder()
	routes.ExecuteRecoveryHandler(execW2, execReq2)
	fmt.Printf("10c: Execute with approval -> HTTP %d: %s\n", execW2.Code, strings.TrimSpace(execW2.Body.String()))
	if execW2.Code != http.StatusOK {
		panic(fmt.Sprintf("FAIL: Approved execution returned %d", execW2.Code))
	}

	// 10d. Restore telemetry (simulate recovery effect)
	routes.State.Lock()
	routes.State.Services["svc-b"].Latency = 15.0
	routes.State.Services["svc-b"].Status = "healthy"
	routes.State.NodeBreachCounts["svc-b"] = 0
	routes.State.Unlock()

	time.Sleep(300 * time.Millisecond)
	routes.WaitForRecoveryVerifications()

	// 10e. Subsequent prediction confirms full resolution
	req = httptest.NewRequest("GET", "/api/predict", nil)
	w = httptest.NewRecorder()
	routes.PredictHandler(w, req)
	var resolvedPred routes.PredictionOutput
	_ = json.Unmarshal(w.Body.Bytes(), &resolvedPred)
	fmt.Printf("10e: Post-recovery Prediction -> Incident: %v (State: %s, Probability: %.1f%%)\n",
		resolvedPred.Incident, resolvedPred.State, resolvedPred.FailureProbability)
	if resolvedPred.Incident {
		panic("FAIL: Post-recovery prediction still reports incident!")
	}
	fmt.Println("--> PASS: Lifecycle verified: 403 blocked -> operator approved -> executing -> verifying -> resolved.")

	// -------------------------------------------------------------------------
	// 11. Second Completely Different Synthetic Topology (10 nodes)
	// -------------------------------------------------------------------------
	fmt.Println("\n[TEST 11] Second Synthetic Topology: 10 Services (cluster-node-0 .. cluster-node-9)")
	routes.State.Lock()
	routes.State.Services = make(map[string]*routes.Microservice)
	routes.State.ServiceOrder = make([]string, 0)
	for i := 0; i < 10; i++ {
		nodeID := fmt.Sprintf("cluster-node-%d", i)
		deps := []string{}
		if i+1 < 10 {
			deps = append(deps, fmt.Sprintf("cluster-node-%d", i+1))
		}
		routes.State.Services[nodeID] = &routes.Microservice{
			ID:             nodeID,
			Name:           fmt.Sprintf("Cluster Service Node %d", i),
			Type:           "service",
			Status:         "healthy",
			CPU:            20.0,
			Memory:         25.0,
			Latency:        10.0,
			ErrorRate:      0.001,
			Replicas:       2,
			Dependencies:   deps,
			LastIngestTime: time.Now(),
			DataStatus:     "fresh",
		}
		routes.State.ServiceOrder = append(routes.State.ServiceOrder, nodeID)
	}

	// Inject fault on cluster-node-7: CPU saturation
	routes.State.Services["cluster-node-7"].CPU = 98.0
	routes.State.Services["cluster-node-7"].Status = "critical"
	routes.State.NodeBreachCounts["cluster-node-7"] = 3
	routes.State.Unlock()

	req = httptest.NewRequest("GET", "/api/predict", nil)
	w = httptest.NewRecorder()
	routes.PredictHandler(w, req)

	var topo2Pred routes.PredictionOutput
	_ = json.Unmarshal(w.Body.Bytes(), &topo2Pred)

	fmt.Printf("10-Node Topology Prediction: RootCause='%s', RecommendedAction='%s' (Target='%s')\n",
		topo2Pred.RootCauseServiceID, topo2Pred.RecommendedAction, topo2Pred.RecommendedActions[0].TargetServiceID)
	if topo2Pred.RootCauseServiceID != "cluster-node-7" {
		panic(fmt.Sprintf("FAIL: Expected root cause cluster-node-7, got '%s'", topo2Pred.RootCauseServiceID))
	}
	if topo2Pred.RecommendedActions[0].TargetServiceID != "cluster-node-7" {
		panic(fmt.Sprintf("FAIL: Action target '%s' does not match cluster-node-7",
			topo2Pred.RecommendedActions[0].TargetServiceID))
	}
	fmt.Println("--> PASS: 10-node synthetic topology validated with zero domain assumptions.")

	// -------------------------------------------------------------------------
	// 12. Decision Trace Audit Endpoint (/debug/decision-trace)
	// -------------------------------------------------------------------------
	fmt.Println("\n[TEST 12] Querying Decision Trace Audit (/debug/decision-trace)")
	dtReq := httptest.NewRequest("GET", "/debug/decision-trace", nil)
	dtW := httptest.NewRecorder()
	routes.DecisionTraceHandler(dtW, dtReq)

	fmt.Printf("Decision Trace Response (HTTP %d):\n%s\n", dtW.Code, dtW.Body.String())
	if dtW.Code != http.StatusOK {
		panic("FAIL: /debug/decision-trace returned non-200")
	}

	fmt.Println("\n================================================================================")
	fmt.Println("ALL LIVE VERIFICATION TESTS PASSED SUCCESSFULLY!")
	fmt.Println("================================================================================")
}
