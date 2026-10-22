package routes

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/aegis/go-services/api-gateway/storage"
)

type IngestSpanItem struct {
	TraceID         string                 `json:"traceId"`
	SpanID          string                 `json:"spanId"`
	ParentSpanID    string                 `json:"parentSpanId,omitempty"`
	ServiceID       string                 `json:"serviceId"`
	SourceServiceID string                 `json:"sourceServiceId,omitempty"`
	TargetServiceID string                 `json:"targetServiceId,omitempty"`
	OperationName   string                 `json:"operationName,omitempty"`
	DurationMs      float64                `json:"durationMs"`
	StatusCode      int                    `json:"statusCode"`
	Attributes      map[string]interface{} `json:"attributes,omitempty"`
	Namespace       string                 `json:"namespace,omitempty"`
	ClusterID       string                 `json:"clusterId,omitempty"`
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

	discoveredCount := 0
	updatedEdges := make([]*storage.TopologyEdge, 0)

	for _, span := range payload.Spans {
		source := span.ServiceID
		if source == "" {
			source = span.SourceServiceID
		}
		target := span.TargetServiceID

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

		ns := span.Namespace
		if ns == "" {
			ns = "default"
		}
		cluster := span.ClusterID
		if cluster == "" {
			cluster = "cluster-primary"
		}

		isErr := span.StatusCode >= 400 || (span.Attributes != nil && span.Attributes["error"] == true)
		edge, err := AuthStorage.AutoDiscoverSpanEdge(r.Context(), source, target, ns, cluster, span.DurationMs, isErr)
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
					Status:       "healthy",
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
					Status:       "healthy",
					Replicas:     1,
					MaxReplicas:  10,
					Dependencies: make([]string, 0),
					History:      make([]ServiceMetric, 0),
				}
			}

			// Add target to source dependencies if not already present
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

type IngestMetricItem struct {
	ServiceID  string  `json:"serviceId"`
	CPU        float64 `json:"cpu"`
	Memory     float64 `json:"memory"`
	Latency    float64 `json:"latency"`
	RPS        float64 `json:"rps"`
	ErrorRate  float64 `json:"errorRate"`
	QueueDepth int     `json:"queueDepth,omitempty"`
	Status     string  `json:"status,omitempty"`
	Namespace  string  `json:"namespace,omitempty"`
	ClusterID  string  `json:"clusterId,omitempty"`
}

type IngestMetricsPayload struct {
	Metrics []IngestMetricItem `json:"metrics"`
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

	nowTime := time.Now().Format("15:04:05")
	updatedCount := 0

	State.mu.Lock()
	for _, m := range payload.Metrics {
		if m.ServiceID == "" {
			continue
		}

		svc, exists := State.Services[m.ServiceID]
		if !exists {
			svc = &Microservice{
				ID:           m.ServiceID,
				Name:         m.ServiceID,
				Type:         "service",
				Status:       "healthy",
				Replicas:     1,
				MaxReplicas:  10,
				Dependencies: make([]string, 0),
				History:      make([]ServiceMetric, 0),
			}
			State.Services[m.ServiceID] = svc
			State.ServiceOrder = append(State.ServiceOrder, m.ServiceID)
		}

		svc.CPU = m.CPU
		svc.Memory = m.Memory
		svc.Latency = m.Latency
		svc.RPS = m.RPS
		svc.ErrorRate = m.ErrorRate
		if m.QueueDepth > 0 {
			svc.QueueDepth = m.QueueDepth
		}
		if m.Status != "" {
			svc.Status = m.Status
		} else {
			if svc.ErrorRate > 0.10 || svc.Latency > 800.0 {
				svc.Status = "critical"
			} else if svc.ErrorRate > 0.02 || svc.Latency > 200.0 {
				svc.Status = "degraded"
			} else {
				svc.Status = "healthy"
			}
		}

		// Append metric history point
		svc.History = append(svc.History, ServiceMetric{
			Timestamp:  nowTime,
			CPU:        svc.CPU,
			Memory:     svc.Memory,
			Latency:    svc.Latency,
			RPS:        svc.RPS,
			ErrorRate:  svc.ErrorRate,
			QueueDepth: svc.QueueDepth,
		})
		if len(svc.History) > 30 {
			svc.History = svc.History[len(svc.History)-30:]
		}

		updatedCount++
	}
	State.mu.Unlock()

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success":          true,
		"metricsIngested":  len(payload.Metrics),
		"servicesUpdated":  updatedCount,
		"message":          fmt.Sprintf("Real metrics updated for %d services", updatedCount),
	})
}
