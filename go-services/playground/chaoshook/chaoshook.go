package chaoshook

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"log"
	"math/rand"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gorilla/mux"
)

type ChaosInjectRequest struct {
	FaultType   string            `json:"fault_type"`
	DurationSec int               `json:"duration_sec"`
	Parameters  map[string]string `json:"parameters"`
}

type ChaosStatusResponse struct {
	NodeID        string  `json:"node_id"`
	ActiveFault   string  `json:"active_fault"`
	Status        string  `json:"status"` // healthy, degraded, critical, down
	CPUPercent    float64 `json:"cpu_percent"`
	MemoryPercent float64 `json:"memory_percent"`
	LatencyMs     float64 `json:"latency_ms"`
	ErrorRate     float64 `json:"error_rate"`
	Killed        bool    `json:"killed"`
	RemainingSec  int     `json:"remaining_seconds"`
}

type Controller struct {
	NodeID          string
	mu              sync.RWMutex
	ActiveFault     string
	Killed          bool
	ExtraLatencyMs  int
	ErrorRate       float64
	CPUPercent      float64
	MemoryPercent   float64
	cancelSpin      context.CancelFunc
	memBuffer       [][]byte
	faultEndTimer   *time.Timer
	faultExpiration time.Time
}

func NewController(nodeID string) *Controller {
	return &Controller{
		NodeID:        nodeID,
		ErrorRate:     0.001,
		CPUPercent:    22.0,
		MemoryPercent: 35.0,
	}
}

func (c *Controller) Inject(faultType string, durationSec int, params map[string]string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Clean up previous fault if running
	c.cleanupLocked()

	if durationSec <= 0 {
		durationSec = 60
	}

	c.ActiveFault = faultType
	c.faultExpiration = time.Now().Add(time.Duration(durationSec) * time.Second)

	switch faultType {
	case "cpu_stress":
		c.CPUPercent = 96.5
		c.ExtraLatencyMs = 180
		ctx, cancel := context.WithCancel(context.Background())
		c.cancelSpin = cancel
		go c.runCPUSpin(ctx)

	case "latency":
		delay := 950
		if val, ok := params["delay_ms"]; ok {
			if d, err := strconv.Atoi(val); err == nil && d > 0 {
				delay = d
			}
		}
		c.ExtraLatencyMs = delay
		c.ErrorRate = 0.08

	case "kill_service":
		c.Killed = true
		c.CPUPercent = 0.0
		c.ErrorRate = 1.0

	case "memory_leak":
		c.MemoryPercent = 94.0
		c.ExtraLatencyMs = 220
		// Allocate small heap blocks
		c.memBuffer = make([][]byte, 100)
		for i := range c.memBuffer {
			c.memBuffer[i] = make([]byte, 1024*1024) // ~100MB
		}

	case "packet_loss":
		c.ErrorRate = 0.25
		c.ExtraLatencyMs = 250

	default:
		c.ExtraLatencyMs = 400
		c.ErrorRate = 0.05
	}

	log.Printf("[%s] CHAOS INJECTED: %s for %ds", c.NodeID, faultType, durationSec)

	c.faultEndTimer = time.AfterFunc(time.Duration(durationSec)*time.Second, func() {
		c.Revert()
	})
}

func (c *Controller) Revert() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cleanupLocked()
	log.Printf("[%s] CHAOS REVERTED: node returned to nominal state", c.NodeID)
}

func (c *Controller) cleanupLocked() {
	if c.cancelSpin != nil {
		c.cancelSpin()
		c.cancelSpin = nil
	}
	if c.faultEndTimer != nil {
		c.faultEndTimer.Stop()
		c.faultEndTimer = nil
	}
	c.memBuffer = nil
	c.ActiveFault = ""
	c.Killed = false
	c.ExtraLatencyMs = 0
	c.ErrorRate = 0.001
	c.CPUPercent = 20.0 + rand.Float64()*5.0
	c.MemoryPercent = 35.0 + rand.Float64()*5.0
}

func (c *Controller) runCPUSpin(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			// Non-blocking CPU burn loop
			h := sha256.New()
			h.Write([]byte("aegis_chaos_cpu_spin_payload"))
			_ = h.Sum(nil)
			time.Sleep(50 * time.Microsecond)
		}
	}
}

func (c *Controller) GetStatus() ChaosStatusResponse {
	c.mu.RLock()
	defer c.mu.RUnlock()

	status := "healthy"
	if c.Killed {
		status = "critical"
	} else if c.ActiveFault != "" {
		if c.CPUPercent > 90.0 || c.ExtraLatencyMs > 800 || c.MemoryPercent > 90.0 {
			status = "critical"
		} else {
			status = "degraded"
		}
	}

	remaining := 0
	if c.ActiveFault != "" && time.Now().Before(c.faultExpiration) {
		remaining = int(time.Until(c.faultExpiration).Seconds())
	}

	lat := 15.0 + float64(c.ExtraLatencyMs)
	return ChaosStatusResponse{
		NodeID:        c.NodeID,
		ActiveFault:   c.ActiveFault,
		Status:        status,
		CPUPercent:    c.CPUPercent,
		MemoryPercent: c.MemoryPercent,
		LatencyMs:     lat,
		ErrorRate:     c.ErrorRate,
		Killed:        c.Killed,
		RemainingSec:  remaining,
	}
}

func (c *Controller) ShouldFail() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.Killed {
		return true
	}
	if c.ErrorRate > 0.001 {
		return rand.Float64() < c.ErrorRate
	}
	return false
}

func (c *Controller) ApplyDelay() {
	c.mu.RLock()
	delay := c.ExtraLatencyMs
	c.mu.RUnlock()
	if delay > 0 {
		time.Sleep(time.Duration(delay) * time.Millisecond)
	}
}

func (c *Controller) RegisterRoutes(r *mux.Router) {
	r.HandleFunc("/chaos/inject", func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var injectReq ChaosInjectRequest
		if err := json.NewDecoder(req.Body).Decode(&injectReq); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		c.Inject(injectReq.FaultType, injectReq.DurationSec, injectReq.Parameters)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"node_id": c.NodeID,
			"fault":   injectReq.FaultType,
			"status":  c.GetStatus(),
		})
	}).Methods("POST", "OPTIONS")

	r.HandleFunc("/chaos/revert", func(w http.ResponseWriter, req *http.Request) {
		c.Revert()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"node_id": c.NodeID,
			"status":  c.GetStatus(),
		})
	}).Methods("POST", "OPTIONS")

	r.HandleFunc("/chaos/status", func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(c.GetStatus())
	}).Methods("GET", "OPTIONS")
}
