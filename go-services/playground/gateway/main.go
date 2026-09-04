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

type Gateway struct {
	NodeID          string
	Port            string
	TopologyFile    string
	StartTime       time.Time
	Topology        *TopologyConfig
	httpClient      *http.Client
	mu              sync.RWMutex
	loadGenStopChan chan struct{}
	loadGenActive   bool

	// Metrics
	routeCounter    *prometheus.CounterVec
	routeDuration   *prometheus.HistogramVec
	activeRequests  prometheus.Gauge
	errorCounter    *prometheus.CounterVec
}

func NewGateway() *Gateway {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	topoPath := os.Getenv("TOPOLOGY_PATH")
	if topoPath == "" {
		topoPath = "/etc/aegis/topology.yaml"
		if _, err := os.Stat(topoPath); os.IsNotExist(err) {
			topoPath = "../../configs/topology.yaml"
		}
	}

	g := &Gateway{
		NodeID:       "gateway",
		Port:         port,
		TopologyFile: topoPath,
		StartTime:    time.Now(),
		httpClient: &http.Client{
			Timeout: 8 * time.Second,
		},
		routeCounter: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "aegis_gateway_routes_total",
				Help: "Total routes processed by API Gateway",
				ConstLabels: prometheus.Labels{
					"node_id": "gateway",
				},
			},
			[]string{"target_node", "status"},
		),
		routeDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name: "aegis_gateway_route_duration_seconds",
				Help: "Duration of route execution",
				ConstLabels: prometheus.Labels{
					"node_id": "gateway",
				},
				Buckets: prometheus.DefBuckets,
			},
			[]string{"target_node"},
		),
		activeRequests: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Name: "aegis_gateway_active_requests",
				Help: "Active requests in gateway",
				ConstLabels: prometheus.Labels{
					"node_id": "gateway",
				},
			},
		),
		errorCounter: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "aegis_gateway_errors_total",
				Help: "Gateway errors",
				ConstLabels: prometheus.Labels{
					"node_id": "gateway",
				},
			},
			[]string{"error_type"},
		),
	}

	prometheus.MustRegister(g.routeCounter)
	prometheus.MustRegister(g.routeDuration)
	prometheus.MustRegister(g.activeRequests)
	prometheus.MustRegister(g.errorCounter)

	g.loadTopology()
	return g
}

func (g *Gateway) loadTopology() {
	data, err := os.ReadFile(g.TopologyFile)
	if err != nil {
		log.Printf("[gateway] Notice: Could not read topology file from %s: %v", g.TopologyFile, err)
		return
	}

	var topo TopologyConfig
	if err := yaml.Unmarshal(data, &topo); err != nil {
		log.Printf("[gateway] Error parsing topology YAML: %v", err)
		return
	}

	g.mu.Lock()
	defer g.mu.Unlock()
	g.Topology = &topo
	log.Printf("[gateway] Successfully loaded topology '%s' with %d nodes", topo.SystemName, len(topo.Nodes))
}

func (g *Gateway) getTargetURL(targetNodeID string) string {
	g.mu.RLock()
	defer g.mu.RUnlock()

	if g.Topology != nil {
		for _, n := range g.Topology.Nodes {
			if n.ID == targetNodeID {
				return fmt.Sprintf("http://%s:%d", n.Host, n.Port)
			}
		}
	}
	return fmt.Sprintf("http://%s:8080", targetNodeID)
}

func (g *Gateway) HealthHandler(w http.ResponseWriter, r *http.Request) {
	uptime := time.Since(g.StartTime).Seconds()
	resp := map[string]interface{}{
		"status":      "UP",
		"node_id":     "gateway",
		"uptime_sec":  uptime,
		"timestamp":   time.Now().Unix(),
		"system_name": "aegis-mesh",
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

func (g *Gateway) TopologyHandler(w http.ResponseWriter, r *http.Request) {
	g.mu.RLock()
	defer g.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	if g.Topology == nil {
		http.Error(w, `{"error": "topology not loaded"}`, http.StatusInternalServerError)
		return
	}
	_ = json.NewEncoder(w).Encode(g.Topology)
}

func (g *Gateway) RouteHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	targetNode := vars["nodeId"]
	if targetNode == "" {
		targetNode = "node-a" // Default entrypoint
	}

	start := time.Now()
	g.activeRequests.Inc()
	defer g.activeRequests.Dec()

	traceID := r.Header.Get("X-Trace-ID")
	if traceID == "" {
		traceID = fmt.Sprintf("gw-trace-%d-%d", time.Now().UnixNano(), rand.Intn(10000))
	}

	payload := ProcessPayload{
		TraceID:      traceID,
		SourceNode:   "gateway",
		PathTraveled: []string{"gateway"},
		Timestamp:    time.Now().Unix(),
	}

	targetURL := fmt.Sprintf("%s/process", g.getTargetURL(targetNode))
	reqBody, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(r.Context(), "POST", targetURL, bytes.NewBuffer(reqBody))
	if err != nil {
		g.errorCounter.WithLabelValues("request_creation_failed").Inc()
		http.Error(w, fmt.Sprintf(`{"error": "failed to build request: %v"}`, err), http.StatusInternalServerError)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Trace-ID", traceID)

	resp, err := g.httpClient.Do(req)
	if err != nil {
		g.errorCounter.WithLabelValues("upstream_call_failed").Inc()
		g.routeCounter.WithLabelValues(targetNode, "502").Inc()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"status":      "ERROR",
			"error":       err.Error(),
			"target_node": targetNode,
			"trace_id":    traceID,
		})
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	duration := time.Since(start)
	g.routeDuration.WithLabelValues(targetNode).Observe(duration.Seconds())
	g.routeCounter.WithLabelValues(targetNode, fmt.Sprintf("%d", resp.StatusCode)).Inc()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(body)
}

func (g *Gateway) StartSyntheticLoadGenerator(interval time.Duration) {
	g.mu.Lock()
	if g.loadGenActive {
		g.mu.Unlock()
		return
	}
	g.loadGenStopChan = make(chan struct{})
	g.loadGenActive = true
	g.mu.Unlock()

	log.Printf("[gateway] Starting synthetic load generator (interval: %v)...", interval)
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-g.loadGenStopChan:
				log.Printf("[gateway] Stopped synthetic load generator")
				return
			case <-ticker.C:
				target := "node-a"
				targetURL := fmt.Sprintf("%s/process", g.getTargetURL(target))
				traceID := fmt.Sprintf("synth-%d", time.Now().UnixNano())
				payload := ProcessPayload{
					TraceID:      traceID,
					SourceNode:   "gateway-synthetic-agent",
					PathTraveled: []string{"gateway"},
					Timestamp:    time.Now().Unix(),
				}
				data, _ := json.Marshal(payload)
				req, err := http.NewRequest("POST", targetURL, bytes.NewBuffer(data))
				if err == nil {
					req.Header.Set("Content-Type", "application/json")
					resp, err := g.httpClient.Do(req)
					if err == nil {
						_ = resp.Body.Close()
					}
				}
			}
		}
	}()
}

func (g *Gateway) SetupRoutes() *mux.Router {
	r := mux.NewRouter()
	r.HandleFunc("/health", g.HealthHandler).Methods("GET")
	r.HandleFunc("/metrics", promhttp.Handler().ServeHTTP).Methods("GET")
	r.HandleFunc("/topology", g.TopologyHandler).Methods("GET")
	r.HandleFunc("/route/{nodeId}", g.RouteHandler).Methods("GET", "POST")
	r.HandleFunc("/route", g.RouteHandler).Methods("GET", "POST")
	return r
}

func main() {
	gw := NewGateway()
	router := gw.SetupRoutes()

	// Optionally start background synthetic load generator if configured
	if os.Getenv("ENABLE_SYNTHETIC_LOAD") == "true" {
		gw.StartSyntheticLoadGenerator(2 * time.Second)
	}

	srv := &http.Server{
		Addr:         ":" + gw.Port,
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
	}

	go func() {
		log.Printf("Starting Aegis Playground Gateway on port %s", gw.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Gateway server error: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Printf("Shutting down gateway...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("Gateway forced to shutdown: %v", err)
	}
	log.Printf("Gateway stopped gracefully")
}
