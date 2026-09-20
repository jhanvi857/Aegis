package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	orchestrator "github.com/aegis/go-services/orchestrator-adapter"
)

type SubmitPlanRequest struct {
	PlanID    string             `json:"plan_id"`
	RiskLevel string             `json:"risk_level"`
	RiskScore float64            `json:"risk_score"`
	Steps     []RecoveryPlanStep `json:"steps"`
}

type ApprovePlanRequest struct {
	PlanID string `json:"plan_id"`
}

func main() {
	log.Println("[recovery-engine] Aegis Recovery Engine initializing (Phase 4)...")

	dockerAdapter := orchestrator.NewDockerAdapter()
	executor := NewExecutor(dockerAdapter)
	approvalGate := NewApprovalGate()
	grpcClient := NewPythonGRPCClient("localhost:50051")

	_ = grpcClient

	mux := http.NewServeMux()

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"UP","service":"recovery-engine","phase":4}`))
	})

	mux.HandleFunc("/plan/submit", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req SubmitPlanRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		autoApproved := approvalGate.RequestApproval(req.PlanID, req.RiskLevel, req.RiskScore, req.Steps)
		w.Header().Set("Content-Type", "application/json")

		if autoApproved {
			result, err := executor.ExecutePlan(context.Background(), req.PlanID, req.Steps)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"status":  "execution_failed",
					"plan_id": req.PlanID,
					"result":  result,
				})
				return
			}
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"status":  "auto_executed",
				"plan_id": req.PlanID,
				"result":  result,
			})
			return
		}

		// Held pending human authorization
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"status":   "held_for_approval",
			"plan_id":  req.PlanID,
			"risk":     req.RiskLevel,
			"message":  "High-risk recovery action requires operator approval.",
		})
	})

	mux.HandleFunc("/plan/pending", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		pending := approvalGate.ListPending()
		_ = json.NewEncoder(w).Encode(pending)
	})

	mux.HandleFunc("/plan/approve", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req ApprovePlanRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		plan, err := approvalGate.GetPlan(req.PlanID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}

		approvalGate.Approve(req.PlanID)
		result, execErr := executor.ExecutePlan(context.Background(), plan.PlanID, plan.Steps)

		w.Header().Set("Content-Type", "application/json")
		if execErr != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"status":  "execution_failed",
				"plan_id": req.PlanID,
				"result":  result,
			})
			return
		}

		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"status":  "approved_and_executed",
			"plan_id": req.PlanID,
			"result":  result,
		})
	})

	srv := &http.Server{Addr: ":8092", Handler: mux}
	go func() {
		log.Println("[recovery-engine] Listening on http://localhost:8092")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("[recovery-engine] HTTP server error: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("[recovery-engine] Shutting down...")
}
