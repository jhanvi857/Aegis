package routes

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/aegis/go-services/api-gateway/storage"
	"github.com/gorilla/mux"
)

// ─── Service Registry Handlers ──────────────────────────────────────────────

func ListRegistryServicesHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	ns := r.URL.Query().Get("namespace")
	cluster := r.URL.Query().Get("clusterId")
	tier := r.URL.Query().Get("tier")

	services, err := AuthStorage.ListServices(r.Context(), ns, cluster)
	if err != nil {
		http.Error(w, `{"error":"Failed to query service registry"}`, http.StatusInternalServerError)
		return
	}

	// Filter by tier if specified
	if tier != "" {
		filtered := make([]*storage.TopologyService, 0)
		for _, s := range services {
			if strings.EqualFold(s.Tier, tier) {
				filtered = append(filtered, s)
			}
		}
		services = filtered
	}

	_ = json.NewEncoder(w).Encode(services)
}

func CreateRegistryServiceHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var svc storage.TopologyService
	if err := json.NewDecoder(r.Body).Decode(&svc); err != nil {
		http.Error(w, `{"error":"Invalid request payload"}`, http.StatusBadRequest)
		return
	}

	if strings.TrimSpace(svc.ID) == "" || strings.TrimSpace(svc.Name) == "" {
		http.Error(w, `{"error":"Service id and name are required"}`, http.StatusBadRequest)
		return
	}

	if svc.Namespace == "" {
		svc.Namespace = "default"
	}
	if svc.ClusterID == "" {
		svc.ClusterID = "cluster-primary"
	}
	if svc.Tier == "" {
		svc.Tier = "tier-2"
	}
	if svc.Capabilities == "" {
		svc.Capabilities = "scale,restart"
	}
	if svc.Status == "" {
		svc.Status = "healthy"
	}
	if svc.Replicas <= 0 {
		svc.Replicas = 1
	}
	if svc.MaxReplicas <= 0 {
		svc.MaxReplicas = 10
	}

	if err := AuthStorage.UpsertService(r.Context(), &svc); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"Failed to register service: %s"}`, err.Error()), http.StatusInternalServerError)
		return
	}

	// Sync in-memory state for immediate visibility by TGNN inference & blast radius
	State.mu.Lock()
	if _, ok := State.Services[svc.ID]; !ok {
		State.ServiceOrder = append(State.ServiceOrder, svc.ID)
	}
	State.Services[svc.ID] = &Microservice{
		ID:           svc.ID,
		Name:         svc.Name,
		Type:         svc.Type,
		Status:       svc.Status,
		Replicas:     svc.Replicas,
		MaxReplicas:  svc.MaxReplicas,
		CPU:          svc.CPU,
		Memory:       svc.Memory,
		Latency:      svc.Latency,
		RPS:          svc.RPS,
		ErrorRate:    svc.ErrorRate,
		QueueDepth:   svc.QueueDepth,
		Version:      svc.Version,
		Uptime:       "0m",
		Dependencies: make([]string, 0),
		History:      make([]ServiceMetric, 0),
	}
	State.mu.Unlock()

	LogAudit(r, "service.create", svc.ID, "LOW", "", fmt.Sprintf("namespace: %s, tier: %s", svc.Namespace, svc.Tier), "SUCCESS", "")

	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(svc)
}

func GetRegistryServiceHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	vars := mux.Vars(r)
	id := vars["id"]

	svc, err := AuthStorage.GetService(r.Context(), id)
	if err != nil {
		http.Error(w, `{"error":"Service not found"}`, http.StatusNotFound)
		return
	}
	_ = json.NewEncoder(w).Encode(svc)
}

func UpdateRegistryServiceHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	vars := mux.Vars(r)
	id := vars["id"]

	existing, err := AuthStorage.GetService(r.Context(), id)
	if err != nil {
		http.Error(w, `{"error":"Service not found"}`, http.StatusNotFound)
		return
	}

	var update storage.TopologyService
	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		http.Error(w, `{"error":"Invalid request payload"}`, http.StatusBadRequest)
		return
	}

	// Apply updates
	if update.Name != "" {
		existing.Name = update.Name
	}
	if update.Type != "" {
		existing.Type = update.Type
	}
	if update.Tier != "" {
		existing.Tier = update.Tier
	}
	if update.Capabilities != "" {
		existing.Capabilities = update.Capabilities
	}
	if update.Owner != "" {
		existing.Owner = update.Owner
	}
	if update.Replicas > 0 {
		existing.Replicas = update.Replicas
	}
	if update.MaxReplicas > 0 {
		existing.MaxReplicas = update.MaxReplicas
	}
	if update.Status != "" {
		existing.Status = update.Status
	}

	if err := AuthStorage.UpsertService(r.Context(), existing); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"Failed to update service: %s"}`, err.Error()), http.StatusInternalServerError)
		return
	}

	// Sync in-memory state
	State.mu.Lock()
	if s, ok := State.Services[id]; ok {
		s.Name = existing.Name
		s.Type = existing.Type
		s.Status = existing.Status
		s.Replicas = existing.Replicas
		s.MaxReplicas = existing.MaxReplicas
	}
	State.mu.Unlock()

	LogAudit(r, "service.update", id, "LOW", "", fmt.Sprintf("updated tier: %s, caps: %s", existing.Tier, existing.Capabilities), "SUCCESS", "")
	_ = json.NewEncoder(w).Encode(existing)
}

func DeleteRegistryServiceHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	vars := mux.Vars(r)
	id := vars["id"]

	if err := AuthStorage.DeleteService(r.Context(), id); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"Failed to delete service: %s"}`, err.Error()), http.StatusInternalServerError)
		return
	}

	// Sync in-memory state
	State.mu.Lock()
	delete(State.Services, id)
	newOrder := make([]string, 0, len(State.ServiceOrder))
	for _, oid := range State.ServiceOrder {
		if oid != id {
			newOrder = append(newOrder, oid)
		}
	}
	State.ServiceOrder = newOrder
	// Remove as dependency from other nodes
	for _, s := range State.Services {
		newDeps := make([]string, 0, len(s.Dependencies))
		for _, dep := range s.Dependencies {
			if dep != id {
				newDeps = append(newDeps, dep)
			}
		}
		s.Dependencies = newDeps
	}
	State.mu.Unlock()

	LogAudit(r, "service.delete", id, "MEDIUM", "", "Deleted service and cascading edges", "SUCCESS", "")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("Service %s and associated dependencies deleted", id),
	})
}

// ─── Topology Edge Handlers ─────────────────────────────────────────────────

func ListRegistryEdgesHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	ns := r.URL.Query().Get("namespace")
	cluster := r.URL.Query().Get("clusterId")

	edges, err := AuthStorage.ListEdges(r.Context(), ns, cluster)
	if err != nil {
		http.Error(w, `{"error":"Failed to query topology edges"}`, http.StatusInternalServerError)
		return
	}
	_ = json.NewEncoder(w).Encode(edges)
}

func CreateRegistryEdgeHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var edge storage.TopologyEdge
	if err := json.NewDecoder(r.Body).Decode(&edge); err != nil {
		http.Error(w, `{"error":"Invalid request payload"}`, http.StatusBadRequest)
		return
	}

	if edge.SourceServiceID == "" || edge.TargetServiceID == "" {
		http.Error(w, `{"error":"sourceServiceId and targetServiceId are required"}`, http.StatusBadRequest)
		return
	}

	if edge.Namespace == "" {
		edge.Namespace = "default"
	}
	if edge.ClusterID == "" {
		edge.ClusterID = "cluster-primary"
	}
	if edge.Relationship == "" {
		edge.Relationship = "calls"
	}
	// Manual addition guarantees manual override immunity and 1.0 confidence
	edge.IsManualOverride = true
	edge.Confidence = 1.0

	if err := AuthStorage.UpsertEdge(r.Context(), &edge); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"Failed to create edge: %s"}`, err.Error()), http.StatusInternalServerError)
		return
	}

	// Sync in-memory state dependencies
	State.mu.Lock()
	if src, ok := State.Services[edge.SourceServiceID]; ok {
		hasDep := false
		for _, dep := range src.Dependencies {
			if dep == edge.TargetServiceID {
				hasDep = true
				break
			}
		}
		if !hasDep {
			src.Dependencies = append(src.Dependencies, edge.TargetServiceID)
		}
	}
	State.mu.Unlock()

	LogAudit(r, "edge.create", edge.SourceServiceID, "LOW", "", fmt.Sprintf("edge -> %s (manual override)", edge.TargetServiceID), "SUCCESS", "")

	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(edge)
}

func DeleteRegistryEdgeHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	vars := mux.Vars(r)
	source := vars["source"]
	target := vars["target"]
	ns := r.URL.Query().Get("namespace")
	cluster := r.URL.Query().Get("clusterId")

	if err := AuthStorage.DeleteEdge(r.Context(), source, target, ns, cluster); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"Failed to delete edge: %s"}`, err.Error()), http.StatusInternalServerError)
		return
	}

	// Remove from in-memory state
	State.mu.Lock()
	if src, ok := State.Services[source]; ok {
		newDeps := make([]string, 0, len(src.Dependencies))
		for _, d := range src.Dependencies {
			if d != target {
				newDeps = append(newDeps, d)
			}
		}
		src.Dependencies = newDeps
	}
	State.mu.Unlock()

	LogAudit(r, "edge.delete", source, "LOW", "", fmt.Sprintf("removed edge to %s", target), "SUCCESS", "")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("Edge %s -> %s deleted", source, target),
	})
}

// ─── Topology Import / Export & Snapshots ───────────────────────────────────

type TopologyImportPayload struct {
	Namespace   string                    `json:"namespace"`
	ClusterID   string                    `json:"clusterId"`
	Description string                    `json:"description"`
	SystemName  string                    `json:"systemName"`
	Services    []storage.TopologyService `json:"services"`
	Nodes       []struct {
		storage.TopologyService
		Dependencies []string `json:"dependencies"`
	} `json:"nodes"`
	Edges       []storage.TopologyEdge    `json:"edges"`
}

func ImportTopologyHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var payload TopologyImportPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, `{"error":"Invalid request payload"}`, http.StatusBadRequest)
		return
	}

	// Support 'nodes' attribute with inline dependencies
	if len(payload.Services) == 0 && len(payload.Nodes) > 0 {
		payload.Services = make([]storage.TopologyService, 0, len(payload.Nodes))
		for _, n := range payload.Nodes {
			svc := n.TopologyService
			payload.Services = append(payload.Services, svc)
			for _, dep := range n.Dependencies {
				payload.Edges = append(payload.Edges, storage.TopologyEdge{
					SourceServiceID:  svc.ID,
					TargetServiceID:  dep,
					Relationship:     "dependency",
					IsManualOverride: true,
					Confidence:       1.0,
				})
			}
		}
	}

	if len(payload.Services) == 0 {
		http.Error(w, `{"error":"No services provided in import payload"}`, http.StatusBadRequest)
		return
	}

	ns := payload.Namespace
	if ns == "" {
		ns = "default"
	}
	cluster := payload.ClusterID
	if cluster == "" {
		cluster = "cluster-primary"
	}

	// 1. Upsert all imported services
	for _, s := range payload.Services {
		s.Namespace = ns
		s.ClusterID = cluster
		if s.Status == "" {
			s.Status = "healthy"
		}
		_ = AuthStorage.UpsertService(r.Context(), &s)
	}

	// 2. Upsert all imported edges
	for _, e := range payload.Edges {
		e.Namespace = ns
		e.ClusterID = cluster
		e.IsManualOverride = true
		e.Confidence = 1.0
		_ = AuthStorage.UpsertEdge(r.Context(), &e)
	}

	// 3. Create versioned snapshot record
	versionNum := int(time.Now().Unix())
	caller := GetCaller(r)
	creator := "operator"
	if caller != nil {
		creator = caller.Email
	}

	desc := payload.Description
	if desc == "" {
		desc = fmt.Sprintf("Dynamic import of %d services and %d edges", len(payload.Services), len(payload.Edges))
	}

	ver := &storage.TopologyVersion{
		ID:          fmt.Sprintf("ver-%d", versionNum),
		Version:     versionNum,
		Description: desc,
		Snapshot: map[string]interface{}{
			"services":  payload.Services,
			"edges":     payload.Edges,
			"namespace": ns,
			"clusterId": cluster,
		},
		CreatedBy: creator,
		CreatedAt: time.Now().UTC(),
	}
	_ = AuthStorage.CreateTopologyVersion(r.Context(), ver)

	// 4. Synchronize in-memory State
	State.mu.Lock()
	State.Services = make(map[string]*Microservice)
	State.ServiceOrder = make([]string, 0, len(payload.Services))

	for _, s := range payload.Services {
		State.ServiceOrder = append(State.ServiceOrder, s.ID)
		State.Services[s.ID] = &Microservice{
			ID:           s.ID,
			Name:         s.Name,
			Type:         s.Type,
			Status:       s.Status,
			Replicas:     s.Replicas,
			MaxReplicas:  s.MaxReplicas,
			CPU:          s.CPU,
			Memory:       s.Memory,
			Latency:      s.Latency,
			RPS:          s.RPS,
			ErrorRate:    s.ErrorRate,
			QueueDepth:   s.QueueDepth,
			Version:      s.Version,
			Uptime:       "0m",
			Dependencies: make([]string, 0),
			History:      make([]ServiceMetric, 0),
		}
	}

	for _, e := range payload.Edges {
		if src, ok := State.Services[e.SourceServiceID]; ok {
			src.Dependencies = append(src.Dependencies, e.TargetServiceID)
		}
	}
	State.mu.Unlock()

	LogAudit(r, "topology.import", cluster, "MEDIUM", "", fmt.Sprintf("imported %d services, %d edges into %s", len(payload.Services), len(payload.Edges), ns), "SUCCESS", "")

	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success":       true,
		"version":       versionNum,
		"servicesCount": len(payload.Services),
		"edgesCount":    len(payload.Edges),
		"namespace":     ns,
		"clusterId":     cluster,
		"message":       "Dynamic topology successfully imported and synchronized",
	})
}

func ListTopologyVersionsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	versions, err := AuthStorage.ListTopologyVersions(r.Context(), 20)
	if err != nil {
		http.Error(w, `{"error":"Failed to query topology versions"}`, http.StatusInternalServerError)
		return
	}
	_ = json.NewEncoder(w).Encode(versions)
}

// ─── System Mode Handler (DEMO_MODE vs Real Mode) ───────────────────────────

type SystemModeResponse struct {
	DemoMode       bool   `json:"demoMode"`
	SimulationMode bool   `json:"simulationMode"`
	ModeLabel      string `json:"modeLabel"`
	ClusterID      string `json:"clusterId"`
	Namespace      string `json:"namespace"`
}

func GetSystemModeHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	State.mu.RLock()
	isDemo := State.DemoMode
	State.mu.RUnlock()

	label := "LIVE CLUSTER (Real Telemetry)"
	if isDemo {
		label = "DEMO MODE (Simulated Telemetry)"
	}

	_ = json.NewEncoder(w).Encode(SystemModeResponse{
		DemoMode:       isDemo,
		SimulationMode: isDemo,
		ModeLabel:      label,
		ClusterID:      "cluster-primary",
		Namespace:      "default",
	})
}

func SetSystemModeHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var req struct {
		DemoMode bool `json:"demoMode"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"Invalid request payload"}`, http.StatusBadRequest)
		return
	}

	State.mu.Lock()
	State.DemoMode = req.DemoMode
	State.mu.Unlock()

	label := "LIVE CLUSTER (Real Telemetry)"
	if req.DemoMode {
		label = "DEMO MODE (Simulated Telemetry)"
	}

	LogAudit(r, "system.mode_change", "system", "MEDIUM", "", fmt.Sprintf("demoMode set to %v", req.DemoMode), "SUCCESS", "")

	_ = json.NewEncoder(w).Encode(SystemModeResponse{
		DemoMode:       req.DemoMode,
		SimulationMode: req.DemoMode,
		ModeLabel:      label,
		ClusterID:      "cluster-primary",
		Namespace:      "default",
	})
}
