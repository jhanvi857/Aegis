package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"gopkg.in/yaml.v3"
)

type TopologyConfig struct {
	Version     string       `yaml:"version"`
	SystemName  string       `yaml:"system_name"`
	Description string       `yaml:"description"`
	Nodes       []NodeConfig `yaml:"nodes"`
}

type NodeConfig struct {
	ID              string            `yaml:"id"`
	Name            string            `yaml:"name"`
	Type            string            `yaml:"type"`
	Host            string            `yaml:"host"`
	Port            int               `yaml:"port"`
	HealthEndpoint  string            `yaml:"health_endpoint"`
	MetricsEndpoint string            `yaml:"metrics_endpoint"`
	Dependencies    []string          `yaml:"dependencies"`
	Metadata        map[string]string `yaml:"metadata"`
}

type ProcessPayload struct {
	TraceID      string            `json:"trace_id"`
	SourceNode   string            `json:"source_node"`
	PathTraveled []string          `json:"path_traveled"`
	Payload      map[string]string `json:"payload,omitempty"`
	Timestamp    int64             `json:"timestamp"`
}

type ProcessResponse struct {
	NodeID       string                     `json:"node_id"`
	TraceID      string                     `json:"trace_id"`
	Status       string                     `json:"status"`
	DurationMs   float64                    `json:"duration_ms"`
	Downstream   map[string]ProcessResponse `json:"downstream,omitempty"`
	PathTraveled []string                   `json:"path_traveled"`
}

type Service struct {
	NodeID          string
	Port            string
	TopologyFile    string
	StartTime       time.Time
	Topology        *TopologyConfig
	DownstreamNodes []string
	httpClient      *http.Client
	mu              sync.RWMutex

	requestCounter  *prometheus.CounterVec
	requestDuration *prometheus.HistogramVec
	activeRequests  prometheus.Gauge
	errorCounter    *prometheus.CounterVec
}

func NewService() *Service {
	nodeID := os.Getenv("NODE_ID")
	if nodeID == "" {
		nodeID = "node-d"
	}
	port := os.Getenv("PORT")
	if port == "" {
		port = "8084"
	}
	topoPath := os.Getenv("TOPOLOGY_PATH")
	if topoPath == "" {
		topoPath = "/etc/aegis/topology.yaml"
		if _, err := os.Stat(topoPath); os.IsNotExist(err) {
			topoPath = "../../configs/topology.yaml"
		}
	}

	s := &Service{
		NodeID:       nodeID,
		Port:         port,
		TopologyFile: topoPath,
		StartTime:    time.Now(),
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
		requestCounter: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "aegis_service_requests_total",
				Help: "Total HTTP requests processed by this node",
				ConstLabels: prometheus.Labels{
					"node_id": nodeID,
				},
			},
			[]string{"endpoint", "status"},
		),
		requestDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name: "aegis_service_request_duration_seconds",
				Help: "Histogram of request durations",
				ConstLabels: prometheus.Labels{
					"node_id": nodeID,
				},
				Buckets: prometheus.DefBuckets,
			},
			[]string{"endpoint"},
		),
		activeRequests: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Name: "aegis_service_active_requests",
				Help: "Currently in-flight requests",
				ConstLabels: prometheus.Labels{
					"node_id": nodeID,
				},
			},
		),
		errorCounter: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "aegis_service_errors_total",
				Help: "Total errors encountered by this node",
				ConstLabels: prometheus.Labels{
					"node_id": nodeID,
				},
			},
			[]string{"error_type"},
		),
	}

	prometheus.MustRegister(s.requestCounter)
	prometheus.MustRegister(s.requestDuration)
	prometheus.MustRegister(s.activeRequests)
	prometheus.MustRegister(s.errorCounter)

	s.loadTopology()
	return s
}

func (s *Service) loadTopology() {
	data, err := os.ReadFile(s.TopologyFile)
	if err != nil {
		log.Printf("[%s] Notice: Could not read topology file from %s: %v", s.NodeID, s.TopologyFile, err)
		downstreamEnv := os.Getenv("DOWNSTREAM_URLS")
		if downstreamEnv != "" {
			s.DownstreamNodes = strings.Split(downstreamEnv, ",")
		}
		return
	}

	var topo TopologyConfig
	if err := yaml.Unmarshal(data, &topo); err != nil {
		log.Printf("[%s] Error parsing topology YAML: %v", s.NodeID, err)
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.Topology = &topo

	for _, node := range topo.Nodes {
		if node.ID == s.NodeID {
			s.DownstreamNodes = node.Dependencies
			log.Printf("[%s] Topology loaded: dependencies -> %v", s.NodeID, s.DownstreamNodes)
			break
		}
	}
}

func (s *Service) getTargetURL(targetNodeID string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.Topology != nil {
		for _, n := range s.Topology.Nodes {
			if n.ID == targetNodeID {
				return fmt.Sprintf("http://%s:%d", n.Host, n.Port)
			}
		}
	}
	return fmt.Sprintf("http://%s:8080", targetNodeID)
}

func (s *Service) HealthHandler(w http.ResponseWriter, r *http.Request) {
	s.requestCounter.WithLabelValues("/health", "200").Inc()
	uptime := time.Since(s.StartTime).Seconds()

	resp := map[string]interface{}{
		"status":      "UP",
		"node_id":     s.NodeID,
		"uptime_sec":  uptime,
		"timestamp":   time.Now().Unix(),
		"downstream":  s.DownstreamNodes,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

func (s *Service) ProcessHandler(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	s.activeRequests.Inc()
	defer s.activeRequests.Dec()

	var payload ProcessPayload
	body, err := io.ReadAll(r.Body)
	if err == nil && len(body) > 0 {
		_ = json.Unmarshal(body, &payload)
	}

	if payload.TraceID == "" {
		payload.TraceID = fmt.Sprintf("trace-%d-%d", time.Now().UnixNano(), rand.Intn(10000))
	}
	payload.PathTraveled = append(payload.PathTraveled, s.NodeID)

	workMs := 5 + rand.Intn(15)
	time.Sleep(time.Duration(workMs) * time.Millisecond)

	downstreamResponses := make(map[string]ProcessResponse)
	s.mu.RLock()
	downstream := make([]string, len(s.DownstreamNodes))
	copy(downstream, s.DownstreamNodes)
	s.mu.RUnlock()

	for _, dep := range downstream {
		depURL := fmt.Sprintf("%s/process", s.getTargetURL(dep))
		depPayload, _ := json.Marshal(payload)

		req, err := http.NewRequestWithContext(r.Context(), "POST", depURL, bytes.NewBuffer(depPayload))
		if err != nil {
			s.errorCounter.WithLabelValues("request_creation_failed").Inc()
			downstreamResponses[dep] = ProcessResponse{
				NodeID: dep,
				Status: fmt.Sprintf("ERROR_CREATING_REQ: %v", err),
			}
			continue
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := s.httpClient.Do(req)
		if err != nil {
			s.errorCounter.WithLabelValues("downstream_call_failed").Inc()
			downstreamResponses[dep] = ProcessResponse{
				NodeID: dep,
				Status: fmt.Sprintf("ERROR_CALLING_DOWNSTREAM: %v", err),
			}
			continue
		}

		var depResp ProcessResponse
		respBody, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err := json.Unmarshal(respBody, &depResp); err == nil {
			downstreamResponses[dep] = depResp
		} else {
			downstreamResponses[dep] = ProcessResponse{
				NodeID: dep,
				Status: fmt.Sprintf("STATUS_%d", resp.StatusCode),
			}
		}
	}

	duration := time.Since(start)
	s.requestDuration.WithLabelValues("/process").Observe(duration.Seconds())
	s.requestCounter.WithLabelValues("/process", "200").Inc()

	res := ProcessResponse{
		NodeID:       s.NodeID,
		TraceID:      payload.TraceID,
		Status:       "OK",
		DurationMs:   float64(duration.Microseconds()) / 1000.0,
		Downstream:   downstreamResponses,
		PathTraveled: payload.PathTraveled,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(res)
}

func (s *Service) InfoHandler(w http.ResponseWriter, r *http.Request) {
	s.requestCounter.WithLabelValues("/info", "200").Inc()
	s.mu.RLock()
	defer s.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"node_id":    s.NodeID,
		"port":       s.Port,
		"downstream": s.DownstreamNodes,
		"topology":   s.Topology,
	})
}

func (s *Service) SetupRoutes() *mux.Router {
	r := mux.NewRouter()
	r.HandleFunc("/health", s.HealthHandler).Methods("GET")
	r.HandleFunc("/metrics", promhttp.Handler().ServeHTTP).Methods("GET")
	r.HandleFunc("/process", s.ProcessHandler).Methods("POST")
	r.HandleFunc("/info", s.InfoHandler).Methods("GET")
	return r
}

func main() {
	svc := NewService()
	router := svc.SetupRoutes()

	srv := &http.Server{
		Addr:         ":" + svc.Port,
		Handler:      router,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	go func() {
		log.Printf("Starting Aegis Service Node [%s] on port %s", svc.NodeID, svc.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server listen error: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Printf("Shutting down node [%s]...", svc.NodeID)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}
	log.Printf("Node [%s] stopped gracefully", svc.NodeID)
}
