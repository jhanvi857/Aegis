package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

type InjectRequest struct {
	FaultType   string            `json:"fault_type"`
	TargetNode  string            `json:"target_node"`
	DurationSec int               `json:"duration_sec"`
	Parameters  map[string]string `json:"parameters"`
}

type StopRequest struct {
	TargetNode string `json:"target_node"`
	FaultID    string `json:"fault_id,omitempty"`
}

func main() {
	log.Println("[chaos-engine] Aegis Chaos Engine initializing...")

	logger := NewEpisodeLogger()
	scheduler := NewChaosScheduler(logger)

	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"UP","service":"chaos-engine"}`))
	})

	http.HandleFunc("/inject", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req InjectRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if req.DurationSec <= 0 {
			req.DurationSec = 60
		}

		go func() {
			_ = scheduler.RunEpisode(context.Background(), req.FaultType, req.TargetNode, time.Duration(req.DurationSec)*time.Second, req.Parameters)
		}()

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"status":      "ACCEPTED",
			"target_node": req.TargetNode,
			"fault_type":  req.FaultType,
			"duration":    req.DurationSec,
			"message":     "Chaos episode initiated and physically applied to service mesh",
		})
	})

	http.HandleFunc("/stop", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req StopRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		target := req.TargetNode
		if target == "" && req.FaultID != "" {
			target = req.FaultID
		}

		if target != "" {
			_ = scheduler.StopEpisode(target)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"status":  "STOPPED",
			"target":  target,
			"message": "Chaos fault terminated and node reverted",
		})
	})

	http.HandleFunc("/active", func(w http.ResponseWriter, r *http.Request) {
		active := scheduler.GetActiveEpisodes()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(active)
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8091"
	}

	srv := &http.Server{Addr: ":" + port}
	go func() {
		log.Printf("[chaos-engine] Listening on port %s", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("[chaos-engine] HTTP server error: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("[chaos-engine] Shutting down...")
}
