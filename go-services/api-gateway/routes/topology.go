package routes

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
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

type TopologyInfo struct {
	ID          string `json:"id"`
	Filename    string `json:"filename"`
	Name        string `json:"name"`
	Description string `json:"description"`
	NodeCount   int    `json:"nodeCount"`
	Active      bool   `json:"active"`
}

func getTopologiesDir() string {
	if envDir := os.Getenv("AEGIS_TOPOLOGIES_DIR"); envDir != "" {
		if fi, err := os.Stat(envDir); err == nil && fi.IsDir() {
			return envDir
		}
	}
	candidates := []string{
		"configs/topologies",
		"../configs/topologies",
		"../../configs/topologies",
		"../../../configs/topologies",
		"/etc/aegis/topologies",
	}
	for _, c := range candidates {
		if fi, err := os.Stat(c); err == nil && fi.IsDir() {
			return c
		}
	}
	if cwd, err := os.Getwd(); err == nil {
		dir := cwd
		for i := 0; i < 5; i++ {
			target := filepath.Join(dir, "configs", "topologies")
			if fi, err := os.Stat(target); err == nil && fi.IsDir() {
				return target
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}
	return "configs/topologies"
}

func TopologyListHandler(w http.ResponseWriter, r *http.Request) {
	dir := getTopologiesDir()
	files, err := os.ReadDir(dir)

	list := make([]TopologyInfo, 0)
	if err == nil {
		for _, f := range files {
			if !f.IsDir() && (strings.HasSuffix(f.Name(), ".yaml") || strings.HasSuffix(f.Name(), ".yml")) {
				fullPath := filepath.Join(dir, f.Name())
				data, readErr := os.ReadFile(fullPath)
				if readErr == nil {
					var t TopologyYAML
					_ = yaml.Unmarshal(data, &t)
					list = append(list, TopologyInfo{
						ID:          f.Name(),
						Filename:    f.Name(),
						Name:        t.SystemName,
						Description: t.Description,
						NodeCount:   len(t.Nodes),
						Active:      false,
					})
				}
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(list)
}

type SelectTopologyRequest struct {
	Filename string `json:"filename"`
}

func TopologySelectHandler(w http.ResponseWriter, r *http.Request) {
	var req SelectTopologyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"Invalid request payload"}`, http.StatusBadRequest)
		return
	}

	targetFile := req.Filename
	var topoPath string
	if targetFile == "topology.yaml" || targetFile == "default" {
		candidates := []string{"configs/topology.yaml", "../configs/topology.yaml", "../../configs/topology.yaml", "../../../configs/topology.yaml"}
		for _, c := range candidates {
			if _, err := os.Stat(c); err == nil {
				topoPath = c
				break
			}
		}
		if topoPath == "" {
			if cwd, err := os.Getwd(); err == nil {
				dir := cwd
				for i := 0; i < 5; i++ {
					target := filepath.Join(dir, "configs", "topology.yaml")
					if fi, err := os.Stat(target); err == nil && !fi.IsDir() {
						topoPath = target
						break
					}
					parent := filepath.Dir(dir)
					if parent == dir {
						break
					}
					dir = parent
				}
			}
		}
	} else {
		dir := getTopologiesDir()
		topoPath = filepath.Join(dir, targetFile)
	}

	if err := LoadTopology(topoPath); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   fmt.Sprintf("Failed to load topology from %s: %v", topoPath, err),
		})
		return
	}

	if store := GetAuthStore(); store != nil {
		if err := ReplaceTopologyRegistry(r.Context(), store); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"success": false,
				"error":   fmt.Sprintf("Topology loaded but registry synchronization failed: %v", err),
			})
			return
		}
	}

	LogAudit(r, "topology.select", "", "", "", fmt.Sprintf("filename: %s", targetFile), "SUCCESS", "")

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success":   true,
		"filename":  targetFile,
		"nodeCount": len(State.ServiceOrder),
		"message":   fmt.Sprintf("Successfully switched active mesh topology to %s", targetFile),
	})
}
