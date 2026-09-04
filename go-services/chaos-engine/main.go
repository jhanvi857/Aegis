package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

type InjectRequest struct {
	FaultType  string            `json:"fault_type"`
	TargetNode string            `json:"target_node"`
	DurationSec int              `json:"duration_sec"`
	Parameters map[string]string `json:"parameters"`
}

func main() {
	log.Println("[chaos-engine] Aegis Chaos Engine initializing (Phase 1 scaffold)...")

	logger := NewEpisodeLogger()
	scheduler := NewChaosScheduler(logger)

	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
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

		go func() {
			_ = scheduler.RunEpisode(r.Context(), req.FaultType, req.TargetNode, time.Duration(req.DurationSec)*time.Second, req.Parameters)
		}()

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"status":"QUEUED","message":"Chaos episode started"}`))
	})

	srv := &http.Server{Addr: ":8091"}
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("[chaos-engine] HTTP server error: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("[chaos-engine] Shutting down...")
}
