package routes

import (
	"encoding/json"
	"net/http"
)

func ChaosHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "STUB",
		"message": "Chaos control endpoint (Phase 1 scaffold)",
	})
}
