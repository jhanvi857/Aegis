package routes

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/aegis/go-services/api-gateway/storage"
)

type ValidationIssue struct {
	Type        string   `json:"type"` // "error" | "warning" | "info"
	Code        string   `json:"code"` // "DUPLICATE_ID", "SELF_LOOP", "ORPHAN_NODE", "CYCLE_DETECTED", "UNMONITORED_DEPENDENCY", "MISSING_TELEMETRY_SOURCE", "MISSING_RECOVERY_BINDING"
	Message     string   `json:"message"`
	ServiceID   string   `json:"serviceId,omitempty"`
	SourceID    string   `json:"sourceId,omitempty"`
	TargetID    string   `json:"targetId,omitempty"`
	Details     []string `json:"details,omitempty"`
}

type ValidationSummary struct {
	ServiceCount int `json:"serviceCount"`
	EdgeCount    int `json:"edgeCount"`
	ErrorCount   int `json:"errorCount"`
	WarningCount int `json:"warningCount"`
	SCCCycles    int `json:"sccCycles"`
}

type ValidationResponse struct {
	Valid   bool              `json:"valid"`
	Issues  []ValidationIssue `json:"issues"`
	Summary ValidationSummary `json:"summary"`
}

type ValidatePayload struct {
	Services []storage.TopologyService `json:"services,omitempty"`
	Edges    []storage.TopologyEdge    `json:"edges,omitempty"`
}

// ValidateTopology analyzes the graph for self-loops, duplicates, orphan nodes, cycles (SCC), unmonitored dependencies, and missing bindings
func ValidateTopology(services []*storage.TopologyService, edges []*storage.TopologyEdge) ValidationResponse {
	issues := make([]ValidationIssue, 0)
	svcMap := make(map[string]*storage.TopologyService)
	idCount := make(map[string]int)

	// 1. Check duplicate IDs & map services
	for _, s := range services {
		idCount[s.ID]++
		if idCount[s.ID] == 2 {
			issues = append(issues, ValidationIssue{
				Type:      "error",
				Code:      "DUPLICATE_ID",
				Message:   "Duplicate service ID detected: " + s.ID,
				ServiceID: s.ID,
			})
		}
		svcMap[s.ID] = s
	}

	inDegree := make(map[string]int)
	outDegree := make(map[string]int)
	adjList := make(map[string][]string)

	for id := range svcMap {
		inDegree[id] = 0
		outDegree[id] = 0
		adjList[id] = make([]string, 0)
	}

	// 2. Validate Edges
	for _, e := range edges {
		// Ignore rejected edges
		if e.State == "rejected" {
			continue
		}

		if e.SourceServiceID == e.TargetServiceID {
			issues = append(issues, ValidationIssue{
				Type:     "error",
				Code:     "SELF_LOOP",
				Message:  "Self-loop dependency detected on service: " + e.SourceServiceID,
				SourceID: e.SourceServiceID,
				TargetID: e.TargetServiceID,
			})
		}

		outDegree[e.SourceServiceID]++
		inDegree[e.TargetServiceID]++
		adjList[e.SourceServiceID] = append(adjList[e.SourceServiceID], e.TargetServiceID)

		// Check unmonitored dependency
		targetSvc, exists := svcMap[e.TargetServiceID]
		if exists {
			if targetSvc.MonitoringMode == "ignore" || targetSvc.TelemetrySource == nil || strings.TrimSpace(targetSvc.TelemetrySource.Type) == "" {
				issues = append(issues, ValidationIssue{
					Type:      "warning",
					Code:      "UNMONITORED_DEPENDENCY",
					Message:   "Service " + e.SourceServiceID + " depends on unmonitored service " + e.TargetServiceID + " (structure preserved as masked node)",
					ServiceID: e.TargetServiceID,
					SourceID:  e.SourceServiceID,
					TargetID:  e.TargetServiceID,
				})
			}
		}
	}

	// 3. Orphan nodes (isolated nodes with degree 0 when total services > 1)
	if len(services) > 1 {
		for _, s := range services {
			if inDegree[s.ID] == 0 && outDegree[s.ID] == 0 {
				issues = append(issues, ValidationIssue{
					Type:      "warning",
					Code:      "ORPHAN_NODE",
					Message:   "Service " + s.ID + " has no incoming or outgoing dependencies",
					ServiceID: s.ID,
				})
			}
		}
	}

	// 4. Missing telemetry sources and recovery bindings
	for _, s := range services {
		if s.MonitoringMode != "ignore" && (s.TelemetrySource == nil || strings.TrimSpace(s.TelemetrySource.Type) == "") {
			issues = append(issues, ValidationIssue{
				Type:      "warning",
				Code:      "MISSING_TELEMETRY_SOURCE",
				Message:   "Service " + s.ID + " has no configured telemetry source (status is Unconnected)",
				ServiceID: s.ID,
			})
		}

		if (s.MonitoringMode == "recommend" || s.MonitoringMode == "auto_remediate") &&
			(s.RecoveryBinding == nil || len(s.RecoveryBinding.AllowedActions) == 0) {
			issues = append(issues, ValidationIssue{
				Type:      "warning",
				Code:      "MISSING_RECOVERY_BINDING",
				Message:   "Service " + s.ID + " has mode '" + s.MonitoringMode + "' without configured recovery binding or allowed actions",
				ServiceID: s.ID,
			})
		}
	}

	// 5. Tarjan's Strongly Connected Components (SCC) to detect cycles
	sccCount := 0
	sccs := tarjanSCC(adjList)
	for _, component := range sccs {
		if len(component) > 1 {
			sccCount++
			issues = append(issues, ValidationIssue{
				Type:    "warning",
				Code:    "CYCLE_DETECTED",
				Message: "Cyclic dependency detected across Strongly Connected Component: [" + strings.Join(component, " -> ") + "]",
				Details: component,
			})
		}
	}

	errCount := 0
	warnCount := 0
	for _, iss := range issues {
		if iss.Type == "error" {
			errCount++
		} else if iss.Type == "warning" {
			warnCount++
		}
	}

	return ValidationResponse{
		Valid:  errCount == 0,
		Issues: issues,
		Summary: ValidationSummary{
			ServiceCount: len(services),
			EdgeCount:    len(edges),
			ErrorCount:   errCount,
			WarningCount: warnCount,
			SCCCycles:    sccCount,
		},
	}
}

// tarjanSCC computes Strongly Connected Components using Tarjan's algorithm
func tarjanSCC(adj map[string][]string) [][]string {
	var index int
	indices := make(map[string]int)
	lowlink := make(map[string]int)
	onStack := make(map[string]bool)
	stack := make([]string, 0)
	var sccs [][]string

	var strongconnect func(v string)
	strongconnect = func(v string) {
		indices[v] = index
		lowlink[v] = index
		index++
		stack = append(stack, v)
		onStack[v] = true

		for _, w := range adj[v] {
			if _, visited := indices[w]; !visited {
				strongconnect(w)
				if lowlink[w] < lowlink[v] {
					lowlink[v] = lowlink[w]
				}
			} else if onStack[w] {
				if indices[w] < lowlink[v] {
					lowlink[v] = indices[w]
				}
			}
		}

		if lowlink[v] == indices[v] {
			var component []string
			for {
				w := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				onStack[w] = false
				component = append(component, w)
				if w == v {
					break
				}
			}
			sccs = append(sccs, component)
		}
	}

	for v := range adj {
		if _, visited := indices[v]; !visited {
			strongconnect(v)
		}
	}

	return sccs
}

// ValidateTopologyHandler handles POST /api/registry/validate
func ValidateTopologyHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var payload ValidatePayload
	_ = json.NewDecoder(r.Body).Decode(&payload)

	var services []*storage.TopologyService
	var edges []*storage.TopologyEdge

	store := GetAuthStore()
	ctx := r.Context()

	if len(payload.Services) > 0 {
		for i := range payload.Services {
			services = append(services, &payload.Services[i])
		}
		for i := range payload.Edges {
			edges = append(edges, &payload.Edges[i])
		}
	} else if store != nil {
		var err error
		services, err = store.ListServices(ctx, "", "")
		if err != nil {
			http.Error(w, `{"error":"Failed to query services for validation"}`, http.StatusInternalServerError)
			return
		}
		edges, err = store.ListEdges(ctx, "", "")
		if err != nil {
			http.Error(w, `{"error":"Failed to query edges for validation"}`, http.StatusInternalServerError)
			return
		}
	}

	resp := ValidateTopology(services, edges)
	_ = json.NewEncoder(w).Encode(resp)
}
