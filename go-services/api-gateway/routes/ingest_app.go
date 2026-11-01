package routes

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"time"
)

type applicationTelemetry struct {
	AppID     string             `json:"app_id"`
	ServiceID string             `json:"service_id"`
	Timestamp time.Time          `json:"timestamp"`
	Metrics   map[string]float64 `json:"metrics"`
	Logs      interface{}        `json:"logs,omitempty"`
}

func IngestApplicationTelemetryHandler(w http.ResponseWriter, r *http.Request) {
	if strings.EqualFold(os.Getenv("USE_KAFKA"), "true") {
		http.Error(w, `{"error":"Kafka ingest is enabled but no Kafka consumer is configured"}`, http.StatusServiceUnavailable)
		return
	}
	caller, err := appKeyCaller(r, "ingest")
	if err != nil {
		http.Error(w, `{"error":"application API key required"}`, http.StatusUnauthorized)
		return
	}
	if !allowIngest(caller.APIKeyObj.ID) {
		http.Error(w, `{"error":"ingest rate limit exceeded"}`, http.StatusTooManyRequests)
		return
	}
	var payload applicationTelemetry
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, `{"error":"invalid telemetry payload"}`, http.StatusBadRequest)
		return
	}
	if payload.AppID == "" {
		payload.AppID = caller.APIKeyObj.AppID
	}
	if payload.AppID != caller.APIKeyObj.AppID {
		http.Error(w, `{"error":"app_id does not match API key"}`, http.StatusForbidden)
		return
	}
	app, err := GetAuthStore().GetApplication(r.Context(), payload.AppID)
	if err != nil {
		http.Error(w, `{"error":"unknown app_id"}`, http.StatusNotFound)
		return
	}
	known := false
	for _, service := range app.Services {
		if service.ID == payload.ServiceID {
			known = true
			break
		}
	}
	if !known {
		http.Error(w, `{"error":"unknown service_id"}`, http.StatusBadRequest)
		return
	}
	if payload.Timestamp.IsZero() {
		payload.Timestamp = time.Now().UTC()
	}
	if payload.Metrics == nil {
		payload.Metrics = map[string]float64{}
	}
	// Feed submitted observations into the existing prediction state. Missing
	// fields stay missing; they are never converted into synthetic zeroes.
	stateKey := payload.AppID + "/" + payload.ServiceID
	State.mu.Lock()
	service := State.Services[stateKey]
	if service == nil {
		service = &Microservice{ID: stateKey, Name: payload.ServiceID, Status: "unknown", DataStatus: "fresh", SignalLastSeen: map[string]time.Time{}}
		State.Services[stateKey] = service
		State.ServiceOrder = append(State.ServiceOrder, stateKey)
	}
	for metric, value := range payload.Metrics {
		switch metric {
		case "cpu":
			service.CPU = value
		case "memory":
			service.Memory = value
		case "latency":
			service.Latency = value
		case "error_rate":
			service.ErrorRate = value
		case "db_connection_wait_ms":
			service.DBWait = value
		case "cache_miss_rate":
			service.CacheMiss = value
		case "queue_depth":
			service.QueueDepth = int(value)
		}
		service.SignalLastSeen[metric] = payload.Timestamp
	}
	service.LastIngestTime = payload.Timestamp
	service.LastSeen = payload.Timestamp
	State.mu.Unlock()
	// Direct synchronous path when USE_KAFKA=false.
	prediction := ComputePredictionOutput()
	State.mu.Lock()
	if State.LatestPredictions == nil {
		State.LatestPredictions = make(map[string]PredictionOutput)
	}
	State.LatestPredictions[stateKey] = prediction
	State.mu.Unlock()
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"accepted": true, "app_id": payload.AppID, "service_id": payload.ServiceID, "timestamp": payload.Timestamp, "prediction": prediction})
}
