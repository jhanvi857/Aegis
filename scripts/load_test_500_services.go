package main

import (
	"fmt"
	"math/rand"
	"runtime"
	"sort"
	"time"
)

// ServiceNode represents a microservice in the 500-node benchmark
type ServiceNode struct {
	ID           string
	Name         string
	Namespace    string
	Tier         string
	Dependencies []string
	Status       string
	ErrorRate    float64
	LatencyP99   float64
}

// Generate500ServiceTopology creates a synthetic 500-service microservice graph
func Generate500ServiceTopology() ([]*ServiceNode, [][2]string) {
	rng := rand.New(rand.NewSource(42))
	tiers := []string{"tier-1-ingress", "tier-2-gateway", "tier-3-business", "tier-4-data", "tier-5-infra"}
	namespaces := []string{"default", "payments", "orders", "catalog", "auth", "analytics", "inventory"}

	nodes := make([]*ServiceNode, 500)
	var edges [][2]string

	// 1. Create nodes across tiers
	for i := 0; i < 500; i++ {
		tierIdx := (i * len(tiers)) / 500
		nsIdx := i % len(namespaces)
		nodeID := fmt.Sprintf("svc-%03d", i+1)

		nodes[i] = &ServiceNode{
			ID:           nodeID,
			Name:         fmt.Sprintf("service-%03d", i+1),
			Namespace:    namespaces[nsIdx],
			Tier:         tiers[tierIdx],
			Dependencies: []string{},
			Status:       "healthy",
			ErrorRate:    0.01 + rng.Float64()*0.02,
			LatencyP99:   15.0 + rng.Float64()*30.0,
		}
	}

	// 2. Generate edges (~1,500 edges across tiers, with 5% feedback/retry cycles)
	for i := 0; i < 500; i++ {
		// Each node has 2 to 6 outbound dependencies
		numDeps := 2 + rng.Intn(4)
		for d := 0; d < numDeps; d++ {
			var targetIdx int
			if rng.Float64() < 0.95 && i < 490 {
				// Forward dependency in next tier
				targetIdx = i + 1 + rng.Intn(min(50, 500-i-1))
			} else {
				// Feedback or retry cycle loop
				targetIdx = max(0, i-1-rng.Intn(min(30, i+1)))
			}

			if targetIdx != i && targetIdx < 500 {
				targetID := nodes[targetIdx].ID
				if !contains(nodes[i].Dependencies, targetID) {
					nodes[i].Dependencies = append(nodes[i].Dependencies, targetID)
					edges = append(edges, [2]string{nodes[i].ID, targetID})
				}
			}
		}
	}

	return nodes, edges
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func contains(arr []string, val string) bool {
	for _, x := range arr {
		if x == val {
			return true
		}
	}
	return false
}

// Tarjan SCC algorithm on 500 nodes
type TarjanSCC struct {
	index      int
	indices    map[string]int
	lowlink    map[string]int
	onStack    map[string]bool
	stack      []string
	components [][]string
	adj        map[string][]string
}

func NewTarjanSCC(nodes []*ServiceNode) *TarjanSCC {
	adj := make(map[string][]string, len(nodes))
	for _, n := range nodes {
		adj[n.ID] = n.Dependencies
	}
	return &TarjanSCC{
		indices:    make(map[string]int, len(nodes)),
		lowlink:    make(map[string]int, len(nodes)),
		onStack:    make(map[string]bool, len(nodes)),
		stack:      make([]string, 0, len(nodes)),
		components: make([][]string, 0),
		adj:        adj,
	}
}

func (t *TarjanSCC) Run(nodes []*ServiceNode) [][]string {
	for _, n := range nodes {
		if _, visited := t.indices[n.ID]; !visited {
			t.strongConnect(n.ID)
		}
	}
	return t.components
}

func (t *TarjanSCC) strongConnect(u string) {
	t.indices[u] = t.index
	t.lowlink[u] = t.index
	t.index++
	t.stack = append(t.stack, u)
	t.onStack[u] = true

	for _, v := range t.adj[u] {
		if _, visited := t.indices[v]; !visited {
			t.strongConnect(v)
			if t.lowlink[v] < t.lowlink[u] {
				t.lowlink[u] = t.lowlink[v]
			}
		} else if t.onStack[v] {
			if t.indices[v] < t.lowlink[u] {
				t.lowlink[u] = t.indices[v]
			}
		}
	}

	if t.lowlink[u] == t.indices[u] {
		var scc []string
		for {
			w := t.stack[len(t.stack)-1]
			t.stack = t.stack[:len(t.stack)-1]
			t.onStack[w] = false
			scc = append(scc, w)
			if w == u {
				break
			}
		}
		t.components = append(t.components, scc)
	}
}

// ComputeBlastRadius calculates k-hop blast radius using BFS
func ComputeBlastRadius(startNode string, revAdj map[string][]string, maxHops int) []string {
	visited := make(map[string]bool)
	queue := []string{startNode}
	visited[startNode] = true
	var blastRadius []string

	for hops := 0; hops < maxHops && len(queue) > 0; hops++ {
		levelSize := len(queue)
		for i := 0; i < levelSize; i++ {
			curr := queue[0]
			queue = queue[1:]

			for _, upstream := range revAdj[curr] {
				if !visited[upstream] {
					visited[upstream] = true
					queue = append(queue, upstream)
					blastRadius = append(blastRadius, upstream)
				}
			}
		}
	}

	return blastRadius
}

func percentile(durations []time.Duration, p float64) time.Duration {
	if len(durations) == 0 {
		return 0
	}
	idx := int(float64(len(durations)-1) * p)
	return durations[idx]
}

func main() {
	fmt.Println("================================================================================")
	fmt.Println("AEGIS HIGH-SCALE LOAD TEST BENCHMARK: 500 MICROSERVICES TOPOLOGY")
	fmt.Println("================================================================================")
	fmt.Println("Target: Ingestion, SCC Condensation, Blast Radius BFS, Memory Efficiency")
	fmt.Println()

	var memStart runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&memStart)

	// Step 1: Ingestion Benchmark
	t0 := time.Now()
	nodes, edges := Generate500ServiceTopology()
	genDuration := time.Since(t0)

	var memAfterGen runtime.MemStats
	runtime.ReadMemStats(&memAfterGen)
	allocMB := float64(memAfterGen.Alloc-memStart.Alloc) / (1024 * 1024)

	fmt.Printf("[1/4] Topology Ingestion Benchmark:\n")
	fmt.Printf("      Nodes Generated:       %d microservices\n", len(nodes))
	fmt.Printf("      Edges Generated:       %d directed dependencies\n", len(edges))
	fmt.Printf("      Ingestion Elapsed:     %v\n", genDuration)
	fmt.Printf("      Memory Allocated:      %.2f MB\n", allocMB)
	fmt.Println()

	// Step 2: Tarjan SCC Condensation Benchmark (100 iterations)
	fmt.Printf("[2/4] SCC Graph Condensation Benchmark (Tarjan's Algorithm, 100 iterations):\n")
	iterations := 100
	sccDurations := make([]time.Duration, iterations)
	var totalSCCs int

	for i := 0; i < iterations; i++ {
		tSCCStart := time.Now()
		tarjan := NewTarjanSCC(nodes)
		sccs := tarjan.Run(nodes)
		sccDurations[i] = time.Since(tSCCStart)
		if i == 0 {
			totalSCCs = len(sccs)
		}
	}

	sort.Slice(sccDurations, func(i, j int) bool { return sccDurations[i] < sccDurations[j] })
	fmt.Printf("      SCC Components:        %d (condensed DAG nodes)\n", totalSCCs)
	fmt.Printf("      Latency p50:           %v\n", percentile(sccDurations, 0.50))
	fmt.Printf("      Latency p95:           %v\n", percentile(sccDurations, 0.95))
	fmt.Printf("      Latency p99:           %v\n", percentile(sccDurations, 0.99))
	fmt.Printf("      Max Latency:           %v\n", sccDurations[len(sccDurations)-1])
	fmt.Println()

	// Step 3: Blast Radius Traversal Benchmark (BFS on reverse adjacency, 500 root causes)
	fmt.Printf("[3/4] Blast Radius Upstream Traversal Benchmark (500 root cause simulations):\n")
	revAdj := make(map[string][]string, len(nodes))
	for _, n := range nodes {
		for _, dep := range n.Dependencies {
			revAdj[dep] = append(revAdj[dep], n.ID)
		}
	}

	blastDurations := make([]time.Duration, len(nodes))
	var totalAffected int

	for i, n := range nodes {
		tBFSStart := time.Now()
		affected := ComputeBlastRadius(n.ID, revAdj, 3)
		blastDurations[i] = time.Since(tBFSStart)
		totalAffected += len(affected)
	}

	sort.Slice(blastDurations, func(i, j int) bool { return blastDurations[i] < blastDurations[j] })
	avgAffected := float64(totalAffected) / float64(len(nodes))

	fmt.Printf("      Average Blast Radius:  %.1f affected services (3-hop window)\n", avgAffected)
	fmt.Printf("      Traversal Latency p50: %v\n", percentile(blastDurations, 0.50))
	fmt.Printf("      Traversal Latency p95: %v\n", percentile(blastDurations, 0.95))
	fmt.Printf("      Traversal Latency p99: %v\n", percentile(blastDurations, 0.99))
	fmt.Println()

	// Step 4: Overall SLA Verification
	fmt.Println("[4/4] Production Readiness Verification Checklist:")
	p99SCC := percentile(sccDurations, 0.99)
	p99Blast := percentile(blastDurations, 0.99)

	sccPass := p99SCC < 10*time.Millisecond
	blastPass := p99Blast < 5*time.Millisecond
	memPass := allocMB < 50.0

	fmt.Printf("      - SCC Condensation p99 < 10ms:       [%s] (%v)\n", statusStr(sccPass), p99SCC)
	fmt.Printf("      - Blast Radius BFS p99 < 5ms:         [%s] (%v)\n", statusStr(blastPass), p99Blast)
	fmt.Printf("      - Memory Allocated < 50MB for 500:   [%s] (%.2f MB)\n", statusStr(memPass), allocMB)

	fmt.Println("================================================================================")
	if sccPass && blastPass && memPass {
		fmt.Println("BENCHMARK RESULT: PASSED (Scale 500 Services Ready)")
	} else {
		fmt.Println("BENCHMARK RESULT: FAILED SLA REQUIREMENTS")
	}
	fmt.Println("================================================================================")
}

func statusStr(b bool) string {
	if b {
		return "PASS"
	}
	return "FAIL"
}
