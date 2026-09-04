package routes

import (
	"encoding/json"
	"net/http"
	"os"
)

func TopologyHandler(w http.ResponseWriter, r *http.Request) {
	topoPath := os.Getenv("TOPOLOGY_PATH")
	if topoPath == "" {
		topoPath = "/etc/aegis/topology.yaml"
		if _, err := os.Stat(topoPath); os.IsNotExist(err) {
			topoPath = "../../configs/topology.yaml"
		}
	}

	data, err := os.ReadFile(topoPath)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"status": "NOT_LOADED",
			"info":   "Topology file not found",
		})
		return
	}

	w.Header().Set("Content-Type", "application/x-yaml")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}
