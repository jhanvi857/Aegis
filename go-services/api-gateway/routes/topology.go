package routes

import (
	"encoding/json"
	"net/http"
)

func TopologyHandler(w http.ResponseWriter, r *http.Request) {
	State.mu.RLock()
	defer State.mu.RUnlock()

	if len(State.RawTopologyYAML) == 0 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"status": "NOT_LOADED",
			"info":   "Topology configuration is not available",
		})
		return
	}

	// Check if client asks for json or yaml
	accept := r.Header.Get("Accept")
	if accept == "application/x-yaml" || accept == "text/yaml" {
		w.Header().Set("Content-Type", "application/x-yaml")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(State.RawTopologyYAML)
		return
	}

	// Return parsed JSON nodes and relationships
	nodes := make([]map[string]interface{}, 0, len(State.ServiceOrder))
	for _, id := range State.ServiceOrder {
		svc := State.Services[id]
		nodes = append(nodes, map[string]interface{}{
			"id":           svc.ID,
			"name":         svc.Name,
			"type":         svc.Type,
			"status":       svc.Status,
			"dependencies": svc.Dependencies,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"system_name": "aegis-mesh",
		"nodes":       nodes,
	})
}
