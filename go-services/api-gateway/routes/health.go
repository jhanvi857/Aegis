package routes

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/aegis/go-services/api-gateway/storage"
)

type ComponentHealth struct {
	Status  string `json:"status"` // UP, DOWN, DEGRADED
	Message string `json:"message,omitempty"`
	Engine  string `json:"engine,omitempty"`
}

type ReadinessResponse struct {
	Status     string                     `json:"status"` // READY, NOT_READY
	Engine     string                     `json:"engine"` // dense_tgnn | sparse_two_stage | heuristic_fallback
	Timestamp  string                     `json:"timestamp"`
	Components map[string]ComponentHealth `json:"components"`
}

// LivenessHandler responds 200 as long as the HTTP process is running
func LivenessHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"UP","service":"aegis-api-gateway"}`))
}

// ReadinessHandler verifies downstream dependencies (Database, gRPC client, state store)
func ReadinessHandler(store storage.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()

		components := make(map[string]ComponentHealth)
		isReady := true
		var mu sync.Mutex
		var wg sync.WaitGroup

		// 1. Check storage health
		wg.Add(1)
		go func() {
			defer wg.Done()
			var comp ComponentHealth
			if store == nil {
				comp = ComponentHealth{Status: "DOWN", Message: "Storage uninitialized"}
			} else {
				// Test by querying 1 user or audit log
				_, err := store.ListAuditLogs(ctx, 1)
				if err != nil {
					comp = ComponentHealth{Status: "DOWN", Message: err.Error()}
				} else {
					comp = ComponentHealth{Status: "UP", Message: "Storage operational"}
				}
			}

			mu.Lock()
			components["storage"] = comp
			if comp.Status != "UP" {
				isReady = false
			}
			mu.Unlock()
		}()

		// 2. Check in-memory control plane state
		wg.Add(1)
		go func() {
			defer wg.Done()
			var comp ComponentHealth
			if State == nil {
				comp = ComponentHealth{Status: "DOWN", Message: "Control plane state not initialized"}
			} else {
				State.mu.RLock()
				serviceCount := len(State.Services)
				State.mu.RUnlock()
				comp = ComponentHealth{
					Status:  "UP",
					Message: "Control plane state active",
				}
				_ = serviceCount
			}

			mu.Lock()
			components["state_engine"] = comp
			if comp.Status != "UP" {
				isReady = false
			}
			mu.Unlock()
		}()

		// 3. Check Python ML gRPC connectivity and loaded engine
		threshold := 20
		if val := os.Getenv("AEGIS_SPARSE_THRESHOLD"); val != "" {
			if t, err := strconv.Atoi(val); err == nil && t > 0 {
				threshold = t
			}
		}

		loadedEngine := "dense_tgnn"
		var svcCount int
		if State != nil {
			State.mu.RLock()
			svcCount = len(State.Services)
			State.mu.RUnlock()
			if svcCount > threshold {
				loadedEngine = "sparse_two_stage"
			}
		}

		if GlobalGRPCClient == nil {
			loadedEngine = "heuristic_fallback"
		}

		wg.Add(1)
		go func() {
			defer wg.Done()
			var comp ComponentHealth
			if GlobalGRPCClient == nil {
				comp = ComponentHealth{
					Status:  "DEGRADED",
					Engine:  "heuristic_fallback",
					Message: "Python ML gRPC client not connected (operating with heuristic_fallback)",
				}
			} else {
				comp = ComponentHealth{
					Status:  "UP",
					Engine:  loadedEngine,
					Message: fmt.Sprintf("Python ML gRPC connected (engine: %s, threshold: %d, services: %d)", loadedEngine, threshold, svcCount),
				}
			}

			mu.Lock()
			components["ml_inference"] = comp
			mu.Unlock()
		}()

		wg.Wait()

		resp := ReadinessResponse{
			Status:     "READY",
			Engine:     loadedEngine,
			Timestamp:  time.Now().UTC().Format(time.RFC3339),
			Components: components,
		}

		if !isReady {
			resp.Status = "NOT_READY"
			w.WriteHeader(http.StatusServiceUnavailable)
		} else {
			w.WriteHeader(http.StatusOK)
		}

		_ = json.NewEncoder(w).Encode(resp)
	}
}
