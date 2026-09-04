package playground_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"gopkg.in/yaml.v3"
)

// TestNode simulates a single service node for unit testing
type TestNode struct {
	ID           string
	Dependencies []string
	Server       *httptest.Server
	TargetURLs   map[string]string
}

func NewTestNode(id string, deps []string) *TestNode {
	n := &TestNode{
		ID:           id,
		Dependencies: deps,
		TargetURLs:   make(map[string]string),
	}

	r := mux.NewRouter()
	r.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"status":     "UP",
			"node_id":    n.ID,
			"downstream": n.Dependencies,
		})
	}).Methods("GET")

	r.HandleFunc("/metrics", promhttp.Handler().ServeHTTP).Methods("GET")

	r.HandleFunc("/process", func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]interface{}
		body, _ := io.ReadAll(r.Body)
		if len(body) > 0 {
			_ = json.Unmarshal(body, &payload)
		} else {
			payload = make(map[string]interface{})
		}

		if payload["trace_id"] == nil {
			payload["trace_id"] = fmt.Sprintf("test-trace-%d", time.Now().UnixNano())
		}

		var path []interface{}
		if p, ok := payload["path_traveled"].([]interface{}); ok {
			path = append(p, n.ID)
		} else {
			path = []interface{}{n.ID}
		}
		payload["path_traveled"] = path

		downstreamResp := make(map[string]interface{})
		client := &http.Client{Timeout: 2 * time.Second}

		for _, dep := range n.Dependencies {
			depURL, ok := n.TargetURLs[dep]
			if !ok {
				continue
			}
			reqBytes, _ := json.Marshal(payload)
			resp, err := client.Post(depURL+"/process", "application/json", bytes.NewBuffer(reqBytes))
			if err != nil {
				downstreamResp[dep] = map[string]string{"error": err.Error()}
				continue
			}
			var depData map[string]interface{}
			_ = json.NewDecoder(resp.Body).Decode(&depData)
			_ = resp.Body.Close()
			downstreamResp[dep] = depData
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"node_id":       n.ID,
			"trace_id":      payload["trace_id"],
			"status":        "OK",
			"path_traveled": path,
			"downstream":    downstreamResp,
		})
	}).Methods("POST")

	n.Server = httptest.NewServer(r)
	return n
}

func TestPlaygroundTopologyMesh(t *testing.T) {
	// Build simulated mesh: Gateway -> Node-A -> (Node-B, Node-C -> Node-D)
	nodeD := NewTestNode("node-d", []string{})
	defer nodeD.Server.Close()

	nodeC := NewTestNode("node-c", []string{"node-d"})
	defer nodeC.Server.Close()
	nodeC.TargetURLs["node-d"] = nodeD.Server.URL

	nodeB := NewTestNode("node-b", []string{})
	defer nodeB.Server.Close()

	nodeA := NewTestNode("node-a", []string{"node-b", "node-c"})
	defer nodeA.Server.Close()
	nodeA.TargetURLs["node-b"] = nodeB.Server.URL
	nodeA.TargetURLs["node-c"] = nodeC.Server.URL

	gw := NewTestNode("gateway", []string{"node-a"})
	defer gw.Server.Close()
	gw.TargetURLs["node-a"] = nodeA.Server.URL

	// 1. Test Health Checks on all nodes
	nodes := []*TestNode{gw, nodeA, nodeB, nodeC, nodeD}
	for _, node := range nodes {
		resp, err := http.Get(node.Server.URL + "/health")
		if err != nil {
			t.Fatalf("Health check failed for node %s: %v", node.ID, err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Errorf("Node %s health returned status %d, expected 200", node.ID, resp.StatusCode)
		}
		var health map[string]interface{}
		_ = json.NewDecoder(resp.Body).Decode(&health)
		_ = resp.Body.Close()
		if health["status"] != "UP" {
			t.Errorf("Node %s health status '%v' != 'UP'", node.ID, health["status"])
		}
	}

	// 2. Test End-to-End Inter-service Routing from Gateway through entire mesh
	reqPayload := map[string]interface{}{
		"trace_id": "test-gw-e2e-trace-001",
	}
	reqBytes, _ := json.Marshal(reqPayload)
	resp, err := http.Post(gw.Server.URL+"/process", "application/json", bytes.NewBuffer(reqBytes))
	if err != nil {
		t.Fatalf("Gateway routing failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Gateway routing returned status %d", resp.StatusCode)
	}

	var result map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&result)
	_ = resp.Body.Close()

	if result["status"] != "OK" {
		t.Fatalf("Gateway response status was '%v', expected 'OK'", result["status"])
	}

	downstreamA, ok := result["downstream"].(map[string]interface{})["node-a"].(map[string]interface{})
	if !ok {
		t.Fatalf("Node-A missing from downstream response: %v", result)
	}

	downstreamAChildren, ok := downstreamA["downstream"].(map[string]interface{})
	if !ok {
		t.Fatalf("Node-A children missing from downstream response: %v", downstreamA)
	}

	if _, ok := downstreamAChildren["node-b"]; !ok {
		t.Errorf("Node-B missing from Node-A downstream response")
	}

	downstreamC, ok := downstreamAChildren["node-c"].(map[string]interface{})
	if !ok {
		t.Errorf("Node-C missing from Node-A downstream response")
	} else {
		downstreamCChildren, ok := downstreamC["downstream"].(map[string]interface{})
		if !ok || downstreamCChildren["node-d"] == nil {
			t.Errorf("Node-D missing from Node-C downstream response: %v", downstreamC)
		}
	}

	t.Logf("Full mesh routing verification succeeded: %v", result)
}

func TestTopologyFileValidity(t *testing.T) {
	topoPath := "../../configs/topology.yaml"
	data, err := os.ReadFile(topoPath)
	if err != nil {
		t.Fatalf("Failed to read topology file: %v", err)
	}

	var config struct {
		Version    string `yaml:"version"`
		SystemName string `yaml:"system_name"`
		Nodes      []struct {
			ID           string   `yaml:"id"`
			Name         string   `yaml:"name"`
			Dependencies []string `yaml:"dependencies"`
		} `yaml:"nodes"`
	}

	if err := yaml.Unmarshal(data, &config); err != nil {
		t.Fatalf("Failed to unmarshal topology.yaml: %v", err)
	}

	if len(config.Nodes) < 5 {
		t.Errorf("Expected at least 5 nodes in topology.yaml, found %d", len(config.Nodes))
	}

	nodeMap := make(map[string]bool)
	for _, n := range config.Nodes {
		nodeMap[n.ID] = true
	}

	for _, n := range config.Nodes {
		for _, dep := range n.Dependencies {
			if !nodeMap[dep] {
				t.Errorf("Node %s has invalid dependency %s (not defined in topology)", n.ID, dep)
			}
		}
	}
}
