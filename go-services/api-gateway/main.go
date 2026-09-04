package main

import (
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/aegis/go-services/api-gateway/routes"
	"github.com/gorilla/mux"
)

func main() {
	log.Println("[api-gateway] Aegis Control Plane API Gateway initializing...")

	r := mux.NewRouter()

	r.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"UP","service":"api-gateway"}`))
	}).Methods("GET")

	r.HandleFunc("/api/topology", routes.TopologyHandler).Methods("GET")
	r.HandleFunc("/api/predict", routes.PredictHandler).Methods("POST")
	r.HandleFunc("/api/chaos", routes.ChaosHandler).Methods("POST")
	r.HandleFunc("/api/recovery", routes.RecoveryHandler).Methods("POST")

	port := os.Getenv("API_GATEWAY_PORT")
	if port == "" {
		port = "8000"
	}

	srv := &http.Server{
		Addr:    ":" + port,
		Handler: r,
	}

	go func() {
		log.Printf("[api-gateway] Control plane listening on port %s", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("[api-gateway] Server error: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("[api-gateway] Shutting down...")
}
