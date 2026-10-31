package routes

import (
	"encoding/json"
	"net/http"

	"github.com/gorilla/mux"
)

func GetServicesHandler(w http.ResponseWriter, r *http.Request) {
	// Dynamically sync edge dependencies from database store if edges exist
	if store := GetAuthStore(); store != nil {
		if dbEdges, err := store.ListEdges(r.Context(), "", ""); err == nil && len(dbEdges) > 0 {
			edgeMap := make(map[string][]string)
			for _, e := range dbEdges {
				if e.State != "rejected" {
					edgeMap[e.SourceServiceID] = append(edgeMap[e.SourceServiceID], e.TargetServiceID)
				}
			}
			State.mu.Lock()
			for id, svc := range State.Services {
				if deps, ok := edgeMap[id]; ok && len(deps) > 0 {
					depSet := make(map[string]struct{})
					for _, d := range svc.Dependencies {
						depSet[d] = struct{}{}
					}
					for _, d := range deps {
						if _, exists := depSet[d]; !exists {
							svc.Dependencies = append(svc.Dependencies, d)
							depSet[d] = struct{}{}
						}
					}
				}
			}
			State.mu.Unlock()
		}
	}

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
