package main

import (
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	log.Println("[recovery-engine] Aegis Recovery Engine initializing (Phase 4 scaffold)...")

	executor := NewExecutor()
	approvalGate := NewApprovalGate()
	grpcClient := NewPythonGRPCClient("localhost:50051")

	_ = executor
	_ = approvalGate
	_ = grpcClient

	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"UP","service":"recovery-engine"}`))
	})

	srv := &http.Server{Addr: ":8092"}
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("[recovery-engine] HTTP server error: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("[recovery-engine] Shutting down...")
}
