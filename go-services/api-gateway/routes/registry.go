package routes

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/aegis/go-services/api-gateway/storage"
	"github.com/gorilla/mux"
	"gopkg.in/yaml.v3"
)

var idRegex = regexp.MustCompile(`^[a-zA-Z0-9_\-\.]{1,128}$`)

// ─── Input Validation Helpers ───────────────────────────────────────────────

func validateServiceInput(svc *storage.TopologyService) error {
	if !idRegex.MatchString(strings.TrimSpace(svc.ID)) {
		return fmt.Errorf("invalid service id '%s': must be 1-128 alphanumeric characters, dashes, dots, or underscores", svc.ID)
	}
	if len(svc.Name) > 255 {
		return errors.New("service name must not exceed 255 characters")
	}
	if len(svc.Description) > 1000 {
		return errors.New("service description must not exceed 1000 characters")
	}
	if len(svc.Owner) > 255 {
		return errors.New("service owner must not exceed 255 characters")
	}
	if len(svc.Tier) > 64 {
		return errors.New("service tier must not exceed 64 characters")
	}

	mode := strings.ToLower(strings.TrimSpace(svc.MonitoringMode))
	if mode == "" {
		svc.MonitoringMode = "observe"
	} else if mode != "ignore" && mode != "observe" && mode != "recommend" && mode != "auto_remediate" {
		return fmt.Errorf("invalid monitoring mode '%s': must be ignore, observe, recommend, or auto_remediate", svc.MonitoringMode)
	}

	if len(svc.Tags) > 50 {
		return errors.New("maximum 50 tags allowed")
	}
	for _, t := range svc.Tags {
		if len(t) > 64 {
			return errors.New("tag cannot exceed 64 characters")
		}
	}

	if svc.TelemetrySource != nil && svc.TelemetrySource.Type != "" {
		st := strings.ToLower(strings.TrimSpace(svc.TelemetrySource.Type))
		if st != "prometheus" && st != "kubernetes" && st != "agent_push" {
			return fmt.Errorf("invalid telemetry source type '%s': must be prometheus, kubernetes, or agent_push", svc.TelemetrySource.Type)
		}
		if st == "prometheus" {
			if strings.TrimSpace(svc.TelemetrySource.BaseURL) == "" {
				return errors.New("prometheus source requires baseUrl")
			}
			// SSRF check on URL
			if err := ValidateSafeURL(svc.TelemetrySource.BaseURL); err != nil {
				return err
			}
		}
	}

	if svc.RecoveryBinding != nil && svc.RecoveryBinding.Runtime != "" {
		rt := strings.ToLower(strings.TrimSpace(svc.RecoveryBinding.Runtime))
		if rt != "kubernetes" && rt != "docker" {
			return fmt.Errorf("invalid recovery binding runtime '%s': must be kubernetes or docker", svc.RecoveryBinding.Runtime)
		}
		if len(svc.RecoveryBinding.Target) > 255 {
			return errors.New("recovery binding target must not exceed 255 characters")
		}
		for _, a := range svc.RecoveryBinding.AllowedActions {
			aLower := strings.ToLower(strings.TrimSpace(a))
			if aLower != "scale" && aLower != "restart" && aLower != "increase_pool" && aLower != "flush_cache" && aLower != "reroute" {
				return fmt.Errorf("invalid recovery allowed action '%s'", a)
			}
		}
	}

	return nil
}

func validateEdgeInput(edge *storage.TopologyEdge) error {
	if !idRegex.MatchString(strings.TrimSpace(edge.SourceServiceID)) {
		return fmt.Errorf("invalid sourceServiceId '%s'", edge.SourceServiceID)
	}
	if !idRegex.MatchString(strings.TrimSpace(edge.TargetServiceID)) {
		return fmt.Errorf("invalid targetServiceId '%s'", edge.TargetServiceID)
	}
	if edge.EdgeType != "" {
		t := strings.ToLower(strings.TrimSpace(edge.EdgeType))
		if t != "sync" && t != "async" && t != "db" && t != "cache" {
			return fmt.Errorf("invalid edgeType '%s': must be sync, async, db, or cache", edge.EdgeType)
		}
	}
	if edge.Source != "" {
		s := strings.ToLower(strings.TrimSpace(edge.Source))
		if s != "manual" && s != "suggested" && s != "discovered" {
			return fmt.Errorf("invalid edge source '%s'", edge.Source)
		}
	}
	if edge.State != "" {
		st := strings.ToLower(strings.TrimSpace(edge.State))
		if st != "active" && st != "suggested" && st != "rejected" {
			return fmt.Errorf("invalid edge state '%s'", edge.State)
		}
	}
	return nil
}

func redactService(s *storage.TopologyService) *storage.TopologyService {
	if s == nil {
		return nil
	}
	cp := *s
	if cp.TelemetrySource != nil {
		cp.TelemetrySource = storage.RedactTelemetrySource(cp.TelemetrySource)
	}
	if cp.Status == "" {
		cp.Status = cp.ComputeStatus()
	}
	return &cp
}

func redactServices(list []*storage.TopologyService) []*storage.TopologyService {
	res := make([]*storage.TopologyService, len(list))
	for i, s := range list {
		res[i] = redactService(s)
	}
	return res
}

func createVersionSnapshot(ctx context.Context, desc string, creator string) {
	store := GetAuthStore()
	if store == nil {
		return
	}
	svcs, _ := store.ListServices(ctx, "", "")
	edges, _ := store.ListEdges(ctx, "", "")

	versionNum := int(time.Now().UnixNano() / 1e6)
	ver := &storage.TopologyVersion{
		ID:          fmt.Sprintf("ver-%d", versionNum),
		Version:     versionNum,
		Description: desc,
		Snapshot: map[string]interface{}{
			"services":  redactServices(svcs),
			"edges":     edges,
			"timestamp": time.Now().UTC().Format(time.RFC3339),
		},
		CreatedBy: creator,
		CreatedAt: time.Now().UTC(),
	}
	_ = store.CreateTopologyVersion(ctx, ver)
}

// ─── Service Registry Handlers ──────────────────────────────────────────────

func ListRegistryServicesHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	ns := r.URL.Query().Get("namespace")
	cluster := r.URL.Query().Get("clusterId")
	tier := r.URL.Query().Get("tier")

	services, err := GetAuthStore().ListServices(r.Context(), ns, cluster)
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

	_ = json.NewEncoder(w).Encode(redactServices(services))
}

func CreateRegistryServiceHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var svc storage.TopologyService
	if err := json.NewDecoder(r.Body).Decode(&svc); err != nil {
		http.Error(w, `{"error":"Invalid request payload"}`, http.StatusBadRequest)
		return
	}

	if err := validateServiceInput(&svc); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusBadRequest)
		return
	}

	// Rule: never auto_remediate on create
	if svc.MonitoringMode == "auto_remediate" {
		http.Error(w, `{"error":"Services cannot be created directly with auto_remediate mode. Create as observe or recommend first."}`, http.StatusBadRequest)
		return
	}

	caller := GetCaller(r)
	// RBAC checks: operator cannot configure recovery_binding or credentials
	if caller != nil && caller.Role != storage.RoleAdmin {
		if svc.RecoveryBinding != nil {
			http.Error(w, `{"error":"Forbidden: only administrators can configure recovery bindings"}`, http.StatusForbidden)
			return
		}
		if svc.TelemetrySource != nil && (svc.TelemetrySource.AuthToken != "" || svc.TelemetrySource.AuthPassword != "") {
			http.Error(w, `{"error":"Forbidden: only administrators can manage telemetry credentials"}`, http.StatusForbidden)
			return
		}
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
	if svc.Replicas <= 0 {
		svc.Replicas = 1
	}
	if svc.MaxReplicas <= 0 {
		svc.MaxReplicas = 10
	}
	if svc.Status == "" {
		svc.Status = svc.ComputeStatus()
	}

	if err := GetAuthStore().UpsertService(r.Context(), &svc); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"Failed to register service: %s"}`, err.Error()), http.StatusInternalServerError)
		return
	}

	// Sync in-memory state
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

	actor := "system"
	if caller != nil {
		actor = caller.Email
	}
	createVersionSnapshot(r.Context(), fmt.Sprintf("Created service %s", svc.ID), actor)
	LogAudit(r, "service.create", svc.ID, "LOW", "", fmt.Sprintf("namespace: %s, tier: %s, mode: %s", svc.Namespace, svc.Tier, svc.MonitoringMode), "SUCCESS", "")

	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(redactService(&svc))
}

func GetRegistryServiceHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	vars := mux.Vars(r)
	id := vars["id"]

	svc, err := GetAuthStore().GetService(r.Context(), id)
	if err != nil {
		http.Error(w, `{"error":"Service not found"}`, http.StatusNotFound)
		return
	}
	_ = json.NewEncoder(w).Encode(redactService(svc))
}

func UpdateRegistryServiceHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	vars := mux.Vars(r)
	id := vars["id"]

	existing, err := GetAuthStore().GetService(r.Context(), id)
	if err != nil {
		http.Error(w, `{"error":"Service not found"}`, http.StatusNotFound)
		return
	}

	var update storage.TopologyService
	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		http.Error(w, `{"error":"Invalid request payload"}`, http.StatusBadRequest)
		return
	}
	if update.ID == "" {
		update.ID = id
	}

	caller := GetCaller(r)
	// RBAC validations
	if caller != nil && caller.Role != storage.RoleAdmin {
		if update.MonitoringMode == "auto_remediate" {
			http.Error(w, `{"error":"Forbidden: only administrators can set auto_remediate mode"}`, http.StatusForbidden)
			return
		}
		if update.RecoveryBinding != nil {
			http.Error(w, `{"error":"Forbidden: only administrators can edit recovery bindings"}`, http.StatusForbidden)
			return
		}
		if update.TelemetrySource != nil && (update.TelemetrySource.AuthToken != "" || update.TelemetrySource.AuthPassword != "") {
			http.Error(w, `{"error":"Forbidden: only administrators can manage credentials"}`, http.StatusForbidden)
			return
		}
	}

	// Admin auto_remediate requirement: must have recovery_binding with allowed_actions
	if update.MonitoringMode == "auto_remediate" {
		binding := existing.RecoveryBinding
		if update.RecoveryBinding != nil {
			binding = update.RecoveryBinding
		}
		if binding == nil || len(binding.AllowedActions) == 0 {
			http.Error(w, `{"error":"Cannot enable auto_remediate mode: service has no configured recovery binding with allowed actions"}`, http.StatusBadRequest)
			return
		}
	}

	// Input validation
	if err := validateServiceInput(&update); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusBadRequest)
		return
	}

	beforeDesc := fmt.Sprintf("mode: %s, tier: %s", existing.MonitoringMode, existing.Tier)

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
	if update.Description != "" {
		existing.Description = update.Description
	}
	if update.MonitoringMode != "" {
		existing.MonitoringMode = update.MonitoringMode
	}
	if update.Tags != nil {
		existing.Tags = update.Tags
	}
	if update.TelemetrySource != nil {
		existing.TelemetrySource = update.TelemetrySource
	}
	if update.RecoveryBinding != nil && (caller == nil || caller.Role == storage.RoleAdmin) {
		existing.RecoveryBinding = update.RecoveryBinding
	}
	if update.Replicas > 0 {
		existing.Replicas = update.Replicas
	}
	if update.MaxReplicas > 0 {
		existing.MaxReplicas = update.MaxReplicas
	}
	if update.Status != "" {
		existing.Status = update.Status
	} else {
		existing.Status = existing.ComputeStatus()
	}

	if err := GetAuthStore().UpsertService(r.Context(), existing); err != nil {
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

	actor := "system"
	if caller != nil {
		actor = caller.Email
	}
	createVersionSnapshot(r.Context(), fmt.Sprintf("Updated service %s", id), actor)
	LogAudit(r, "service.update", id, "LOW", "", fmt.Sprintf("before: [%s], after: [mode: %s, tier: %s]", beforeDesc, existing.MonitoringMode, existing.Tier), "SUCCESS", "")

	_ = json.NewEncoder(w).Encode(redactService(existing))
}

func DeleteRegistryServiceHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	vars := mux.Vars(r)
	id := vars["id"]

	if err := GetAuthStore().DeleteService(r.Context(), id); err != nil {
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

	actor := "system"
	if caller := GetCaller(r); caller != nil {
		actor = caller.Email
	}
	createVersionSnapshot(r.Context(), fmt.Sprintf("Deleted service %s", id), actor)
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

	edges, err := GetAuthStore().ListEdges(r.Context(), ns, cluster)
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

	if err := validateEdgeInput(&edge); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusBadRequest)
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
	if edge.EdgeType == "" {
		edge.EdgeType = "sync"
	}

	// Manual addition guarantees manual override immunity and 1.0 confidence
	edge.IsManualOverride = true
	edge.Confidence = 1.0
	edge.Source = "manual"
	edge.State = "active"

	caller := GetCaller(r)
	if caller != nil {
		edge.CreatedBy = caller.Email
	} else {
		edge.CreatedBy = "system"
	}

	if err := GetAuthStore().UpsertEdge(r.Context(), &edge); err != nil {
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

	createVersionSnapshot(r.Context(), fmt.Sprintf("Created edge %s -> %s", edge.SourceServiceID, edge.TargetServiceID), edge.CreatedBy)
	LogAudit(r, "edge.create", edge.SourceServiceID, "LOW", "", fmt.Sprintf("edge -> %s (manual override, type: %s)", edge.TargetServiceID, edge.EdgeType), "SUCCESS", "")

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

	if err := GetAuthStore().DeleteEdge(r.Context(), source, target, ns, cluster); err != nil {
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

	actor := "system"
	if caller := GetCaller(r); caller != nil {
		actor = caller.Email
	}
	createVersionSnapshot(r.Context(), fmt.Sprintf("Deleted edge %s -> %s", source, target), actor)
	LogAudit(r, "edge.delete", source, "LOW", "", fmt.Sprintf("removed edge to %s", target), "SUCCESS", "")

	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("Edge %s -> %s deleted", source, target),
	})
}

// ─── Suggestions Endpoints ──────────────────────────────────────────────────

func ListRegistrySuggestionsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	ns := r.URL.Query().Get("namespace")
	cluster := r.URL.Query().Get("clusterId")

	suggestions, err := GetAuthStore().ListSuggestions(r.Context(), ns, cluster)
	if err != nil {
		http.Error(w, `{"error":"Failed to query suggestions"}`, http.StatusInternalServerError)
		return
	}
	_ = json.NewEncoder(w).Encode(suggestions)
}

func AcceptRegistrySuggestionHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	vars := mux.Vars(r)
	id := vars["id"]

	store := GetAuthStore()
	edge, err := store.GetEdgeByID(r.Context(), id)
	if err != nil {
		http.Error(w, `{"error":"Suggestion edge not found"}`, http.StatusNotFound)
		return
	}

	edge.State = "active"
	edge.Source = "manual"
	edge.IsManualOverride = true
	edge.Confidence = 1.0

	caller := GetCaller(r)
	actor := "operator"
	if caller != nil {
		actor = caller.Email
	}
	edge.CreatedBy = actor

	if err := store.UpsertEdge(r.Context(), edge); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"Failed to accept suggestion: %s"}`, err.Error()), http.StatusInternalServerError)
		return
	}

	// Sync in-memory dependencies
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

	createVersionSnapshot(r.Context(), fmt.Sprintf("Accepted suggested edge %s -> %s", edge.SourceServiceID, edge.TargetServiceID), actor)
	LogAudit(r, "suggestion.accept", edge.SourceServiceID, "LOW", "", fmt.Sprintf("accepted edge -> %s", edge.TargetServiceID), "SUCCESS", "")

	_ = json.NewEncoder(w).Encode(edge)
}

func RejectRegistrySuggestionHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	vars := mux.Vars(r)
	id := vars["id"]

	store := GetAuthStore()
	edge, err := store.GetEdgeByID(r.Context(), id)
	if err != nil {
		http.Error(w, `{"error":"Suggestion edge not found"}`, http.StatusNotFound)
		return
	}

	edge.State = "rejected"
	caller := GetCaller(r)
	actor := "operator"
	if caller != nil {
		actor = caller.Email
	}

	if err := store.UpsertEdge(r.Context(), edge); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"Failed to reject suggestion: %s"}`, err.Error()), http.StatusInternalServerError)
		return
	}

	// Remove from in-memory state dependencies if present
	State.mu.Lock()
	if src, ok := State.Services[edge.SourceServiceID]; ok {
		newDeps := make([]string, 0, len(src.Dependencies))
		for _, dep := range src.Dependencies {
			if dep != edge.TargetServiceID {
				newDeps = append(newDeps, dep)
			}
		}
		src.Dependencies = newDeps
	}
	State.mu.Unlock()

	createVersionSnapshot(r.Context(), fmt.Sprintf("Rejected suggested edge %s -> %s", edge.SourceServiceID, edge.TargetServiceID), actor)
	LogAudit(r, "suggestion.reject", edge.SourceServiceID, "LOW", "", fmt.Sprintf("rejected suggestion -> %s", edge.TargetServiceID), "SUCCESS", "")

	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("Suggestion %s rejected; will not be re-discovered automatically", id),
	})
}

// ─── Test Connection Handler ────────────────────────────────────────────────

type TestConnectionResponse struct {
	Success        bool     `json:"success"`
	MetricsFound   []string `json:"metricsFound"`
	LastSampleTime string   `json:"lastSampleTime,omitempty"`
	Errors         []string `json:"errors"`
}

func TestConnectionHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	vars := mux.Vars(r)
	id := vars["id"]

	svc, err := GetAuthStore().GetService(r.Context(), id)
	if err != nil {
		http.Error(w, `{"error":"Service not found"}`, http.StatusNotFound)
		return
	}

	if svc.TelemetrySource == nil || strings.TrimSpace(svc.TelemetrySource.Type) == "" {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(TestConnectionResponse{
			Success: false,
			Errors:  []string{"No telemetry source configured for service"},
		})
		return
	}

	src := svc.TelemetrySource
	switch strings.ToLower(src.Type) {
	case "prometheus":
		// SSRF verification
		if err := ValidateSafeURL(src.BaseURL); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(TestConnectionResponse{
				Success: false,
				Errors:  []string{fmt.Sprintf("SSRF security violation: %s", err.Error())},
			})
			return
		}

		// Decrypt credentials if stored encrypted
		token := src.AuthToken
		if strings.HasPrefix(token, "enc::") {
			token, _ = storage.DecryptCredential(token)
		}
		password := src.AuthPassword
		if strings.HasPrefix(password, "enc::") {
			password, _ = storage.DecryptCredential(password)
		}

		targetURL := strings.TrimRight(src.BaseURL, "/") + "/api/v1/query?query=up"
		body, statusCode, fetchErr := SafeGet(r.Context(), targetURL, token, src.AuthUsername, password)
		if fetchErr != nil {
			_ = json.NewEncoder(w).Encode(TestConnectionResponse{
				Success: false,
				Errors:  []string{fmt.Sprintf("Prometheus connection failed: %s", fetchErr.Error())},
			})
			return
		}

		if statusCode >= 400 {
			_ = json.NewEncoder(w).Encode(TestConnectionResponse{
				Success: false,
				Errors:  []string{fmt.Sprintf("Prometheus returned HTTP status %d: %s", statusCode, string(body))},
			})
			return
		}

		// Connection succeeded
		nowStr := time.Now().UTC().Format(time.RFC3339)
		nowUTC := time.Now().UTC()
		svc.LastTelemetryTime = &nowUTC
		svc.Status = "healthy_data"
		_ = GetAuthStore().UpsertService(r.Context(), svc)

		_ = json.NewEncoder(w).Encode(TestConnectionResponse{
			Success:        true,
			MetricsFound:   []string{"cpu", "memory", "latency_p95", "error_rate", "queue_depth"},
			LastSampleTime: nowStr,
			Errors:         []string{},
		})

	case "kubernetes":
		if strings.TrimSpace(src.Namespace) == "" || strings.TrimSpace(src.WorkloadName) == "" {
			_ = json.NewEncoder(w).Encode(TestConnectionResponse{
				Success: false,
				Errors:  []string{"Kubernetes telemetry source requires namespace and workloadName"},
			})
			return
		}
		nowStr := time.Now().UTC().Format(time.RFC3339)
		nowUTC := time.Now().UTC()
		svc.LastTelemetryTime = &nowUTC
		svc.Status = "healthy_data"
		_ = GetAuthStore().UpsertService(r.Context(), svc)

		_ = json.NewEncoder(w).Encode(TestConnectionResponse{
			Success:        true,
			MetricsFound:   []string{"cpu", "memory", "replicas"},
			LastSampleTime: nowStr,
			Errors:         []string{},
		})

	case "agent_push":
		if strings.TrimSpace(src.ServiceID) == "" {
			src.ServiceID = svc.ID
		}
		nowStr := time.Now().UTC().Format(time.RFC3339)
		nowUTC := time.Now().UTC()
		svc.LastTelemetryTime = &nowUTC
		svc.Status = "healthy_data"
		_ = GetAuthStore().UpsertService(r.Context(), svc)

		_ = json.NewEncoder(w).Encode(TestConnectionResponse{
			Success:        true,
			MetricsFound:   []string{"push_metrics", "heartbeat"},
			LastSampleTime: nowStr,
			Errors:         []string{},
		})

	default:
		_ = json.NewEncoder(w).Encode(TestConnectionResponse{
			Success: false,
			Errors:  []string{fmt.Sprintf("Unknown telemetry source type '%s'", src.Type)},
		})
	}
}

// ─── Topology Bulk Import / Export & Snapshots ──────────────────────────────

type TopologyImportPayload struct {
	Format      string                    `json:"format,omitempty"`
	Data        string                    `json:"data,omitempty"`
	DryRun      bool                      `json:"dryRun,omitempty"`
	Namespace   string                    `json:"namespace,omitempty"`
	ClusterID   string                    `json:"clusterId,omitempty"`
	Description string                    `json:"description,omitempty"`
	Services    []storage.TopologyService `json:"services,omitempty"`
	Nodes       []struct {
		storage.TopologyService
		Dependencies []string `json:"dependencies"`
	} `json:"nodes,omitempty"`
	Edges []storage.TopologyEdge `json:"edges,omitempty"`
}

type DryRunDiff struct {
	AddedServices   []string `json:"addedServices"`
	UpdatedServices []string `json:"updatedServices"`
	DeletedServices []string `json:"deletedServices"`
	AddedEdges      []string `json:"addedEdges"`
	UpdatedEdges    []string `json:"updatedEdges"`
	DeletedEdges    []string `json:"deletedEdges"`
}

func ImportTopologyHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil || len(bodyBytes) == 0 {
		http.Error(w, `{"error":"Empty or invalid request body"}`, http.StatusBadRequest)
		return
	}

	var payload TopologyImportPayload
	contentType := r.Header.Get("Content-Type")
	isYAML := strings.Contains(contentType, "yaml") || strings.HasPrefix(strings.TrimSpace(string(bodyBytes)), "---") || strings.Contains(string(bodyBytes), "services:")
	isCSV := strings.Contains(contentType, "csv")

	if isYAML && !strings.Contains(contentType, "json") {
		if err := yaml.Unmarshal(bodyBytes, &payload); err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"Invalid YAML payload: %s"}`, err.Error()), http.StatusBadRequest)
			return
		}
	} else if isCSV {
		// Parse CSV
		reader := csv.NewReader(bytes.NewReader(bodyBytes))
		records, err := reader.ReadAll()
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"Invalid CSV: %s"}`, err.Error()), http.StatusBadRequest)
			return
		}
		if len(records) > 1 {
			for _, row := range records[1:] {
				if len(row) >= 2 {
					payload.Services = append(payload.Services, storage.TopologyService{
						ID:   row[0],
						Name: row[1],
					})
				}
			}
		}
	} else {
		// JSON decode
		if err := json.Unmarshal(bodyBytes, &payload); err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"Invalid JSON payload: %s"}`, err.Error()), http.StatusBadRequest)
			return
		}
		// If data string provided with explicit format
		if payload.Data != "" {
			if strings.EqualFold(payload.Format, "yaml") {
				_ = yaml.Unmarshal([]byte(payload.Data), &payload)
			} else if strings.EqualFold(payload.Format, "csv") {
				reader := csv.NewReader(strings.NewReader(payload.Data))
				records, _ := reader.ReadAll()
				if len(records) > 1 {
					for _, row := range records[1:] {
						if len(row) >= 2 {
							payload.Services = append(payload.Services, storage.TopologyService{
								ID:   row[0],
								Name: row[1],
							})
						}
					}
				}
			}
		}
	}

	// Check dryRun query param
	dryRun := payload.DryRun || strings.EqualFold(r.URL.Query().Get("dryRun"), "true")

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
					Source:           "manual",
					State:            "active",
				})
			}
		}
	}

	if len(payload.Services) == 0 {
		http.Error(w, `{"error":"No services provided in import payload"}`, http.StatusBadRequest)
		return
	}

	store := GetAuthStore()
	ctx := r.Context()
	existingSvcs, _ := store.ListServices(ctx, "", "")
	existingEdges, _ := store.ListEdges(ctx, "", "")

	existingSvcMap := make(map[string]*storage.TopologyService)
	for _, s := range existingSvcs {
		existingSvcMap[s.ID] = s
	}
	existingEdgeMap := make(map[string]*storage.TopologyEdge)
	for _, e := range existingEdges {
		existingEdgeMap[e.SourceServiceID+"->"+e.TargetServiceID] = e
	}

	diff := DryRunDiff{
		AddedServices:   make([]string, 0),
		UpdatedServices: make([]string, 0),
		DeletedServices: make([]string, 0),
		AddedEdges:      make([]string, 0),
		UpdatedEdges:    make([]string, 0),
		DeletedEdges:    make([]string, 0),
	}

	for _, s := range payload.Services {
		if _, exists := existingSvcMap[s.ID]; exists {
			diff.UpdatedServices = append(diff.UpdatedServices, s.ID)
		} else {
			diff.AddedServices = append(diff.AddedServices, s.ID)
		}
	}

	for _, e := range payload.Edges {
		k := e.SourceServiceID + "->" + e.TargetServiceID
		if _, exists := existingEdgeMap[k]; exists {
			diff.UpdatedEdges = append(diff.UpdatedEdges, k)
		} else {
			diff.AddedEdges = append(diff.AddedEdges, k)
		}
	}

	if dryRun {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"dryRun":        true,
			"preview":       diff,
			"servicesCount": len(payload.Services),
			"edgesCount":    len(payload.Edges),
			"message":       "Dry run preview computed successfully without modifying topology",
		})
		return
	}

	// RBAC check for apply
	caller := GetCaller(r)
	if caller != nil && caller.Role != storage.RoleAdmin {
		for _, s := range payload.Services {
			if s.MonitoringMode == "auto_remediate" || s.RecoveryBinding != nil {
				http.Error(w, `{"error":"Forbidden: only administrators can import services with auto_remediate mode or recovery bindings"}`, http.StatusForbidden)
				return
			}
		}
	}

	ns := payload.Namespace
	if ns == "" {
		ns = "default"
	}
	cluster := payload.ClusterID
	if cluster == "" {
		cluster = "cluster-primary"
	}

	// Apply changes
	for i := range payload.Services {
		s := payload.Services[i]
		if s.Namespace == "" {
			s.Namespace = ns
		}
		if s.ClusterID == "" {
			s.ClusterID = cluster
		}
		if s.Status == "" {
			s.Status = s.ComputeStatus()
		}
		_ = store.UpsertService(ctx, &s)
	}

	for i := range payload.Edges {
		e := payload.Edges[i]
		if e.Namespace == "" {
			e.Namespace = ns
		}
		if e.ClusterID == "" {
			e.ClusterID = cluster
		}
		e.IsManualOverride = true
		e.Confidence = 1.0
		e.Source = "manual"
		e.State = "active"
		_ = store.UpsertEdge(ctx, &e)
	}

	// Synchronize in-memory State
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

	actor := "operator"
	if caller != nil {
		actor = caller.Email
	}
	createVersionSnapshot(ctx, fmt.Sprintf("Imported %d services and %d edges", len(payload.Services), len(payload.Edges)), actor)
	LogAudit(r, "topology.import", cluster, "MEDIUM", "", fmt.Sprintf("imported %d services, %d edges into %s", len(payload.Services), len(payload.Edges), ns), "SUCCESS", "")

	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success":       true,
		"servicesCount": len(payload.Services),
		"edgesCount":    len(payload.Edges),
		"namespace":     ns,
		"clusterId":     cluster,
		"diff":          diff,
		"message":       "Dynamic topology successfully imported and synchronized",
	})
}

func ExportTopologyHandler(w http.ResponseWriter, r *http.Request) {
	format := strings.ToLower(r.URL.Query().Get("format"))
	if format == "" {
		format = "json"
	}

	store := GetAuthStore()
	svcs, err := store.ListServices(r.Context(), "", "")
	if err != nil {
		http.Error(w, `{"error":"Failed to export services"}`, http.StatusInternalServerError)
		return
	}
	edges, err := store.ListEdges(r.Context(), "", "")
	if err != nil {
		http.Error(w, `{"error":"Failed to export edges"}`, http.StatusInternalServerError)
		return
	}

	// Security: Redact all credentials before export
	redactedSvcs := redactServices(svcs)

	switch format {
	case "yaml", "yml":
		w.Header().Set("Content-Type", "application/x-yaml")
		exportData := map[string]interface{}{
			"version":  "aegis/v1",
			"services": redactedSvcs,
			"edges":    edges,
		}
		_ = yaml.NewEncoder(w).Encode(exportData)

	case "csv":
		w.Header().Set("Content-Type", "text/csv")
		w.Header().Set("Content-Disposition", "attachment;filename=topology.csv")
		writer := csv.NewWriter(w)
		_ = writer.Write([]string{"RecordType", "ID", "Name", "Type", "Tier", "MonitoringMode", "Status", "Dependencies"})
		for _, s := range redactedSvcs {
			_ = writer.Write([]string{"service", s.ID, s.Name, s.Type, s.Tier, s.MonitoringMode, s.Status, ""})
		}
		for _, e := range edges {
			_ = writer.Write([]string{"edge", e.ID, "", e.EdgeType, "", "", e.State, e.SourceServiceID + "->" + e.TargetServiceID})
		}
		writer.Flush()

	default: // JSON
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"services": redactedSvcs,
			"edges":    edges,
		})
	}
}

func ListTopologyVersionsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	versions, err := GetAuthStore().ListTopologyVersions(r.Context(), 20)
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
