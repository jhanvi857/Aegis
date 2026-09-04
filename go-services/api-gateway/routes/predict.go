package routes

import (
	"encoding/json"
	"net/http"
)

func PredictHandler(w http.ResponseWriter, r *http.Request) {
	// Proxies to Python gRPC server in Phase 3
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "STUB",
		"message": "Prediction proxy endpoint (Phase 3 scaffold)",
		"predictions": []interface{}{},
	})
}
