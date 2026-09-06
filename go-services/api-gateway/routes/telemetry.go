package routes

import (
	"encoding/json"
	"net/http"

	"github.com/gorilla/mux"
)

func GetServicesHandler(w http.ResponseWriter, r *http.Request) {
	State.mu.RLock()
	defer State.mu.RUnlock()

	services := make([]*Microservice, 0, len(State.ServiceOrder))
	for _, id := range State.ServiceOrder {
		if svc, ok := State.Services[id]; ok {
			services = append(services, svc)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(services)
}

func GetServiceByIDHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	State.mu.RLock()
	defer State.mu.RUnlock()

	svc, exists := State.Services[id]
	if !exists {
		http.Error(w, `{"error":"Service not found"}`, http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(svc)
}

func GetAlertsHandler(w http.ResponseWriter, r *http.Request) {
	State.mu.RLock()
	defer State.mu.RUnlock()

	alerts := State.Alerts
	if alerts == nil {
		alerts = make([]LiveAlert, 0)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(alerts)
}

func GetLogsHandler(w http.ResponseWriter, r *http.Request) {
	State.mu.RLock()
	defer State.mu.RUnlock()

	logs := State.Logs
	if logs == nil {
		logs = make([]LogEntry, 0)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(logs)
}

func GetExperimentsHandler(w http.ResponseWriter, r *http.Request) {
	State.mu.RLock()
	defer State.mu.RUnlock()

	experiments := State.Experiments
	if experiments == nil {
		experiments = make([]Experiment, 0)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(experiments)
}
