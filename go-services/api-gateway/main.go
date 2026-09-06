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

// corsMiddleware enables cross-origin requests from the React frontend
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, Accept")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func main() {
	log.Println("[api-gateway] Aegis Control Plane API Gateway initializing...")

	// Initialize dynamic state from topology configuration
	routes.InitState()

	r := mux.NewRouter()

	// Apply CORS
	r.Use(corsMiddleware)

	// Health Check
	r.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"UP","service":"api-gateway"}`))
	}).Methods("GET", "OPTIONS")

	// Topology Routes
	r.HandleFunc("/api/topology", routes.TopologyHandler).Methods("GET", "OPTIONS")

	// Telemetry Routes
	r.HandleFunc("/api/telemetry/services", routes.GetServicesHandler).Methods("GET", "OPTIONS")
	r.HandleFunc("/api/telemetry/services/{id}", routes.GetServiceByIDHandler).Methods("GET", "OPTIONS")
	r.HandleFunc("/api/telemetry/alerts", routes.GetAlertsHandler).Methods("GET", "OPTIONS")
	r.HandleFunc("/api/telemetry/logs", routes.GetLogsHandler).Methods("GET", "OPTIONS")
	r.HandleFunc("/api/experiments", routes.GetExperimentsHandler).Methods("GET", "OPTIONS")

	// Chaos Engine Routes
	r.HandleFunc("/api/chaos", routes.ChaosHandler).Methods("GET", "POST", "OPTIONS")
	r.HandleFunc("/api/chaos/inject", routes.ChaosInjectHandler).Methods("POST", "OPTIONS")
	r.HandleFunc("/api/chaos/stop/{id}", routes.ChaosStopHandler).Methods("POST", "OPTIONS")
	r.HandleFunc("/api/chaos/active", routes.ChaosActiveHandler).Methods("GET", "OPTIONS")

	// Prediction Routes
	r.HandleFunc("/api/predict", routes.PredictHandler).Methods("GET", "POST", "OPTIONS")
	r.HandleFunc("/api/prediction/latest", routes.GetLatestPredictionHandler).Methods("GET", "OPTIONS")
	r.HandleFunc("/api/prediction/history", routes.GetPredictionHistoryHandler).Methods("GET", "OPTIONS")

	// Autonomous Recovery Routes
	r.HandleFunc("/api/recovery", routes.RecoveryHandler).Methods("GET", "POST", "OPTIONS")
	r.HandleFunc("/api/recovery/recommendations", routes.GetRecommendationsHandler).Methods("GET", "OPTIONS")
	r.HandleFunc("/api/recovery/execute", routes.ExecuteRecoveryHandler).Methods("POST", "OPTIONS")
	r.HandleFunc("/api/recovery/history", routes.GetRecoveryHistoryHandler).Methods("GET", "OPTIONS")

	port := os.Getenv("API_GATEWAY_PORT")
	if port == "" {
		port = "8000"
	}

	srv := &http.Server{
		Addr:    ":" + port,
		Handler: r,
	}

	go func() {
		log.Printf("[api-gateway] Control plane listening on http://localhost:%s", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("[api-gateway] Server error: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("[api-gateway] Shutting down...")
}
