package routes

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"sort"
	"time"

	"github.com/aegis/go-services/api-gateway/storage"
)

type IngestSpanItem struct {
	TraceID            string                 `json:"traceId"`
	SpanID             string                 `json:"spanId"`
	ParentSpanID       string                 `json:"parentSpanId,omitempty"`
	ServiceID          string                 `json:"serviceId"`
	SourceServiceID    string                 `json:"sourceServiceId,omitempty"`
	TargetServiceID    string                 `json:"targetServiceId,omitempty"`
	OperationName      string                 `json:"operationName,omitempty"`
	StartTimeUnixNano  int64                  `json:"startTimeUnixNano,omitempty"`
	EndTimeUnixNano    int64                  `json:"endTimeUnixNano,omitempty"`
	DurationMs         float64                `json:"durationMs"`
	StatusCode         int                    `json:"statusCode"`
	Attributes         map[string]interface{} `json:"attributes,omitempty"`
	Namespace          string                 `json:"namespace,omitempty"`
	ClusterID          string                 `json:"clusterId,omitempty"`
	ResourceAttributes map[string]string      `json:"resourceAttributes,omitempty"`
}

type IngestSpansPayload struct {
	Spans []IngestSpanItem `json:"spans"`
}

// IngestSpansHandler ingests OpenTelemetry trace spans to auto-discover topology edges
func IngestSpansHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var payload IngestSpansPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, `{"error":"Invalid request payload"}`, http.StatusBadRequest)
		return
	}

	if len(payload.Spans) == 0 {
		http.Error(w, `{"error":"No spans provided in payload"}`, http.StatusBadRequest)
		return
	}
	rememberRecentSpans(payload.Spans)

	discoveredCount := 0
	updatedEdges := make([]*storage.TopologyEdge, 0)

	for _, span := range payload.Spans {
		source := span.ServiceID
		if source == "" {
			source = span.SourceServiceID
		}
		target := span.TargetServiceID
		EnsureDiscoveredService(r.Context(), source, "otlp-traces", span.Namespace, span.ClusterID, span.ResourceAttributes)

		// If targetServiceId is in attributes, extract it
		if target == "" && span.Attributes != nil {
			if t, ok := span.Attributes["peer.service"].(string); ok {
				target = t
			} else if t, ok := span.Attributes["service.name"].(string); ok && t != source {
				target = t
			}
		}

		if source == "" || target == "" || source == target {
			continue
		}
		EnsureDiscoveredService(r.Context(), target, "otlp-traces", span.Namespace, span.ClusterID, nil)

		ns := span.Namespace
		if ns == "" {
			ns = "default"
		}
		cluster := span.ClusterID
		if cluster == "" {
			cluster = "cluster-primary"
		}

		isErr := span.StatusCode >= 400 || (span.Attributes != nil && span.Attributes["error"] == true)
		store := GetAuthStore()
		if store == nil {
			continue
		}
		edge, err := store.AutoDiscoverSpanEdge(r.Context(), source, target, ns, cluster, span.DurationMs, isErr)
		if err == nil && edge != nil {
			discoveredCount++
			updatedEdges = append(updatedEdges, edge)

			// Update in-memory state graph immediately
			State.mu.Lock()
			if _, exists := State.Services[source]; !exists {
				State.ServiceOrder = append(State.ServiceOrder, source)
				State.Services[source] = &Microservice{
					ID:           source,
					Name:         source,
					Type:         "service",
					Status:       "unknown",
					DataStatus:   "unknown",
					Source:       "otlp",
					Namespace:    ns,
					ClusterID:    cluster,
					Replicas:     1,
					MaxReplicas:  10,
					Dependencies: make([]string, 0),
					History:      make([]ServiceMetric, 0),
				}
			}
			if _, exists := State.Services[target]; !exists {
				State.ServiceOrder = append(State.ServiceOrder, target)
				State.Services[target] = &Microservice{
					ID:           target,
					Name:         target,
					Type:         "service",
					Status:       "unknown",
					DataStatus:   "unknown",
					Source:       "otlp",
					Namespace:    ns,
					ClusterID:    cluster,
					Replicas:     1,
					MaxReplicas:  10,
					Dependencies: make([]string, 0),
					History:      make([]ServiceMetric, 0),
				}
			}

			// Add target to source dependencies unless rejected
			if edge.State != "rejected" {
				srcSvc := State.Services[source]
				hasDep := false
				for _, d := range srcSvc.Dependencies {
					if d == target {
						hasDep = true
						break
					}
				}
				if !hasDep {
					srcSvc.Dependencies = append(srcSvc.Dependencies, target)
				}
			}
			State.mu.Unlock()
		}
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success":         true,
		"spansIngested":   len(payload.Spans),
		"edgesDiscovered": discoveredCount,
		"message":         fmt.Sprintf("Ingested %d spans; auto-discovered/updated %d dependency edges", len(payload.Spans), discoveredCount),
	})
}

func rememberRecentSpans(spans []IngestSpanItem) {
	if len(spans) == 0 {
		return
	}
	State.mu.Lock()
	defer State.mu.Unlock()
	seen := make(map[string]struct{}, len(State.Traces)+len(spans))
	for _, existing := range State.Traces {
		if existing.TraceID != "" && existing.SpanID != "" {
			seen[existing.TraceID+":"+existing.SpanID] = struct{}{}
		}
	}
	for _, span := range spans {
		key := span.TraceID + ":" + span.SpanID
		if span.TraceID != "" && span.SpanID != "" {
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
		}
		State.Traces = append(State.Traces, span)
	}
	if len(State.Traces) > 10000 {
		State.Traces = State.Traces[len(State.Traces)-10000:]
	}
}

type IngestMetricItem struct {
	ServiceID          string               `json:"serviceId"`
	CPU                float64              `json:"cpu"`
	Memory             float64              `json:"memory"`
	Latency            float64              `json:"latency"`
	RPS                float64              `json:"rps"`
	ErrorRate          float64              `json:"errorRate"`
	QueueDepth         int                  `json:"queueDepth,omitempty"`
	Status             string               `json:"status,omitempty"`
	Source             string               `json:"source,omitempty"`
	Namespace          string               `json:"namespace,omitempty"`
	ClusterID          string               `json:"clusterId,omitempty"`
	ResourceAttributes map[string]string    `json:"resourceAttributes,omitempty"`
	ObservedTimestamps map[string]time.Time `json:"observedTimestamps,omitempty"`
	DBWait             float64              `json:"dbWait,omitempty"`
	// Observed preserves sparse telemetry semantics for OTLP and clients that
	// explicitly submit partial service measurements.
	Observed map[string]float64 `json:"observed,omitempty"`
}

type IngestMetricsPayload struct {
	Metrics []IngestMetricItem `json:"metrics"`
}

// EnsureDiscoveredService creates identity records without claiming that metric
// data was received. Logs and traces can discover a service independently.
func EnsureDiscoveredService(ctx context.Context, id, source, namespace, clusterID string, resourceAttributes map[string]string) {
	if id == "" {
		return
	}
	store := GetAuthStore()
	if store != nil {
		registered, err := store.GetService(ctx, id)
		if err != nil {
			if namespace == "" {
				namespace = "default"
			}
			if clusterID == "" {
				clusterID = "default"
			}
			_ = store.UpsertService(ctx, &storage.TopologyService{ID: id, Name: id, Type: "service", Namespace: namespace, ClusterID: clusterID, Tier: "tier-2", Replicas: 1, MaxReplicas: 10, MonitoringMode: "observe", Source: source, DataStatus: "unknown", ResourceAttributes: resourceAttributes})
		} else {
			changed := false
			if registered.ResourceAttributes == nil {
				registered.ResourceAttributes = map[string]string{}
			}
			for key, value := range resourceAttributes {
				if registered.ResourceAttributes[key] != value {
					registered.ResourceAttributes[key] = value
					changed = true
				}
			}
			if source != "" && registered.Source != source {
				registered.Source = source
				changed = true
			}
			if namespace != "" && registered.Namespace != namespace {
				registered.Namespace = namespace
				changed = true
			}
			if clusterID != "" && registered.ClusterID != clusterID {
				registered.ClusterID = clusterID
				changed = true
			}
			if changed {
				_ = store.UpsertService(ctx, registered)
			}
		}
	}
	State.mu.Lock()
	defer State.mu.Unlock()
	if existing, ok := State.Services[id]; ok {
		if source != "" {
			existing.Source = source
		}
		if namespace != "" {
			existing.Namespace = namespace
		}
		if clusterID != "" {
			existing.ClusterID = clusterID
		}
		if len(resourceAttributes) > 0 {
			existing.ResourceAttributes = resourceAttributes
		}
	} else {
		State.Services[id] = &Microservice{ID: id, Name: id, Type: "service", Status: "unknown", DataStatus: "unknown", Replicas: 1, MaxReplicas: 10, Namespace: namespace, ClusterID: clusterID, ResourceAttributes: resourceAttributes, Source: source, Dependencies: []string{}, History: []ServiceMetric{}, SignalLastSeen: map[string]time.Time{}}
		State.ServiceOrder = append(State.ServiceOrder, id)
	}
}

// IngestMetricsHandler ingests Prometheus / agent metric batches into real state
func IngestMetricsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var raw json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		http.Error(w, `{"error":"Invalid request payload"}`, http.StatusBadRequest)
		return
	}

	var payload IngestMetricsPayload
	if err := json.Unmarshal(raw, &payload); err == nil && len(payload.Metrics) > 0 {
		// Successfully parsed batch payload
	} else {
		// Check for single metric object payload
		var single IngestMetricItem
		if errSingle := json.Unmarshal(raw, &single); errSingle == nil && single.ServiceID != "" {
			payload.Metrics = []IngestMetricItem{single}
		}
	}

	if len(payload.Metrics) == 0 {
		http.Error(w, `{"error":"No metrics provided in payload"}`, http.StatusBadRequest)
		return
	}

	updatedCount := IngestMetricsBatch(r.Context(), payload.Metrics)

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success":         true,
		"metricsIngested": len(payload.Metrics),
		"servicesUpdated": updatedCount,
		"message":         fmt.Sprintf("Real metrics updated for %d services", updatedCount),
	})
}

// IngestMetricsBatch processes and writes metric items into the live control-plane state
func IngestMetricsBatch(ctx context.Context, metrics []IngestMetricItem) int {
	updatedCount := 0

	State.mu.Lock()
	for _, m := range metrics {
		if m.ServiceID == "" {
			continue
		}

		store := GetAuthStore()
		var regSvc *storage.TopologyService
		if store != nil {
			regSvc, _ = store.GetService(ctx, m.ServiceID)
		}

		// Drop data for ignore-mode services
		if regSvc != nil && regSvc.MonitoringMode == "ignore" {
			continue
		}

		isDemo := State.DemoMode

		// Telemetry is authoritative for discovery: unknown services are registered
		// instead of being discarded when a static registry exists.
		if regSvc == nil && store != nil && !isDemo {
			namespace, clusterID := m.Namespace, m.ClusterID
			if namespace == "" {
				namespace = "default"
			}
			if clusterID == "" {
				clusterID = "default"
			}
			regSvc = &storage.TopologyService{ID: m.ServiceID, Name: m.ServiceID, Type: "service", Namespace: namespace, ClusterID: clusterID, Tier: "tier-2", Replicas: 1, MaxReplicas: 10, MonitoringMode: "observe", Source: m.Source, ResourceAttributes: m.ResourceAttributes}
			_ = store.UpsertService(ctx, regSvc)
		}

		svc, exists := State.Services[m.ServiceID]
		if !exists {
			svc = &Microservice{
				ID:           m.ServiceID,
				Name:         m.ServiceID,
				Type:         "service",
				Status:       "unknown",
				Replicas:     1,
				MaxReplicas:  10,
				Dependencies: make([]string, 0),
				History:      make([]ServiceMetric, 0),
			}
			State.Services[m.ServiceID] = svc
			State.ServiceOrder = append(State.ServiceOrder, m.ServiceID)
		}

		observed := m.Observed
		if observed == nil {
			// Backwards-compatible legacy batch contract. OTLP uses Observed so
			// missing instruments never become synthetic zero measurements.
			observed = map[string]float64{
				"cpu": m.CPU, "memory": m.Memory, "latency": m.Latency,
				"rps": m.RPS, "error_rate": m.ErrorRate,
				"queue_depth": float64(m.QueueDepth), "db_wait": m.DBWait,
			}
		}
		arrivalTime := time.Now().UTC()
		accepted := make(map[string]float64, len(observed))
		signalTimes := make(map[string]time.Time, len(observed))
		if svc.SignalLastSeen == nil {
			svc.SignalLastSeen = make(map[string]time.Time)
		}
		for signal, value := range observed {
			if math.IsNaN(value) || math.IsInf(value, 0) {
				continue
			}
			signalTime := m.ObservedTimestamps[signal]
			if signalTime.IsZero() {
				signalTime = arrivalTime
			}
			if signalTime.After(arrivalTime.Add(30 * time.Second)) {
				continue
			}
			if signalTime.After(arrivalTime) {
				signalTime = arrivalTime
			}
			if previous := svc.SignalLastSeen[signal]; !previous.IsZero() && !signalTime.After(previous) {
				continue
			}
			accepted[signal] = value
			signalTimes[signal] = signalTime
			svc.SignalLastSeen[signal] = signalTime
		}
		observed = accepted
		if len(observed) == 0 {
			if m.Status == "offline" {
				svc.Status = "offline"
				svc.DataStatus = "fresh"
				svc.LastSeen = arrivalTime
				svc.LastIngestTime = arrivalTime
				if regSvc != nil && store != nil {
					regSvc.Status = "offline"
					regSvc.DataStatus = "fresh"
					regSvc.LastSeen = &arrivalTime
					regSvc.LastTelemetryTime = &arrivalTime
					_ = store.UpsertService(ctx, regSvc)
				}
			} else if m.Source == "prom" && m.Status == "unknown" {
				// A successful Prometheus `up` sample is fresh reachability evidence,
				// but it is not application-health evidence. Keep health unknown and
				// advance freshness without inventing a metric value.
				svc.Status = "unknown"
				svc.DataStatus = "fresh"
				svc.LastSeen = arrivalTime
				svc.LastIngestTime = arrivalTime
				svc.Source = m.Source
				if regSvc != nil && store != nil {
					regSvc.Status = "unknown"
					regSvc.DataStatus = "fresh"
					regSvc.LastSeen = &arrivalTime
					regSvc.LastTelemetryTime = &arrivalTime
					regSvc.Source = m.Source
					_ = store.UpsertService(ctx, regSvc)
				}
			}
			updatedCount++
			continue
		}
		svc.ObservedSignals = svc.ObservedSignals[:0]
		for signal := range svc.SignalLastSeen {
			svc.ObservedSignals = append(svc.ObservedSignals, signal)
		}
		sort.Strings(svc.ObservedSignals)
		if value, ok := observed["cpu"]; ok {
			svc.CPU = value
		}
		if value, ok := observed["memory"]; ok {
			svc.Memory = value
		}
		if value, ok := observed["latency"]; ok {
			svc.Latency = value
		}
		if value, ok := observed["rps"]; ok {
			svc.RPS = value
		}
		if value, ok := observed["error_rate"]; ok {
			svc.ErrorRate = value
		}
		if value, ok := observed["queue_depth"]; ok {
			svc.QueueDepth = int(value)
		}
		if value, ok := observed["db_wait"]; ok {
			svc.DBWait = value
		}
		svc.LastSeen = arrivalTime
		for _, signalTime := range signalTimes {
			if signalTime.After(svc.LastIngestTime) {
				svc.LastIngestTime = signalTime
			}
		}
		freshnessWindow := 2 * time.Duration(getIncidentGateConfig().ScrapeIntervalSec) * time.Second
		if freshnessWindow <= 0 {
			freshnessWindow = 2 * time.Second
		}
		svc.DataStatus = "fresh"
		if arrivalTime.Sub(svc.LastIngestTime) > freshnessWindow {
			svc.DataStatus = "stale"
			svc.Status = "unknown"
		}
		if regSvc != nil && store != nil {
			latestSignalTime := svc.LastIngestTime.UTC()
			regSvc.LastTelemetryTime = &latestSignalTime
			regSvc.LastSeen = &arrivalTime
			regSvc.DataStatus = svc.DataStatus
			if m.Source != "" {
				regSvc.Source = m.Source
			}
			if m.Namespace != "" {
				regSvc.Namespace = m.Namespace
			}
			if m.ClusterID != "" {
				regSvc.ClusterID = m.ClusterID
			}
			if len(m.ResourceAttributes) > 0 {
				if regSvc.ResourceAttributes == nil {
					regSvc.ResourceAttributes = make(map[string]string)
				}
				for key, value := range m.ResourceAttributes {
					regSvc.ResourceAttributes[key] = value
				}
			}
		}
		if m.Source != "" {
			svc.Source = m.Source
		}
		if m.Namespace != "" {
			svc.Namespace = m.Namespace
		}
		if m.ClusterID != "" {
			svc.ClusterID = m.ClusterID
		}
		if len(m.ResourceAttributes) > 0 {
			svc.ResourceAttributes = make(map[string]string, len(m.ResourceAttributes))
			for key, value := range m.ResourceAttributes {
				svc.ResourceAttributes[key] = value
			}
		}
		if regSvc != nil && store != nil {
			if value, ok := observed["cpu"]; ok {
				regSvc.CPU = value
			}
			if value, ok := observed["memory"]; ok {
				regSvc.Memory = value
			}
			if value, ok := observed["latency"]; ok {
				regSvc.Latency = value
			}
			if value, ok := observed["rps"]; ok {
				regSvc.RPS = value
			}
			if value, ok := observed["error_rate"]; ok {
				regSvc.ErrorRate = value
			}
			if value, ok := observed["queue_depth"]; ok {
				regSvc.QueueDepth = int(value)
			}
			_ = store.UpsertService(ctx, regSvc)
		}
		if m.Status != "" && m.Status != "unknown" {
			svc.Status = m.Status
		} else {
			_, hasErrors := observed["error_rate"]
			_, hasLatency := observed["latency"]
			if !hasErrors && !hasLatency {
				svc.Status = "unknown"
			} else if (hasErrors && svc.ErrorRate > 0.10) || (hasLatency && svc.Latency > 800.0) {
				svc.Status = "critical"
			} else if (hasErrors && svc.ErrorRate > 0.02) || (hasLatency && svc.Latency > 200.0) {
				svc.Status = "degraded"
			} else {
				svc.Status = "healthy"
			}
		}

		// Append metric history point
		svc.History = append(svc.History, ServiceMetric{
			Timestamp:  svc.LastIngestTime.UTC().Format(time.RFC3339Nano),
			CPU:        svc.CPU,
			Memory:     svc.Memory,
			Latency:    svc.Latency,
			RPS:        svc.RPS,
			ErrorRate:  svc.ErrorRate,
			QueueDepth: svc.QueueDepth, ObservedSignals: append([]string(nil), svc.ObservedSignals...),
		})
		if len(svc.History) > 30 {
			svc.History = svc.History[len(svc.History)-30:]
		}

		updatedCount++
	}
	State.mu.Unlock()
	return updatedCount
}
