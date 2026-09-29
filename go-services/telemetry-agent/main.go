package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	log.Println("[telemetry-agent] Aegis Telemetry Agent initializing (Phase 1 scaffold)...")

	kafkaBrokers := os.Getenv("KAFKA_BROKERS")
	if kafkaBrokers == "" {
		kafkaBrokers = "localhost:9092"
	}
	publisher := NewKafkaPublisher([]string{kafkaBrokers}, "aegis-telemetry")
	collector := NewCollector([]string{"gateway", "node-a", "node-b", "node-c", "node-d"}, 2*time.Second, publisher)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	collector.Start(ctx)

	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"UP","service":"telemetry-agent"}`))
	})

	srv := &http.Server{Addr: ":8090"}
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("[telemetry-agent] HTTP server error: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("[telemetry-agent] Shutting down...")
}
