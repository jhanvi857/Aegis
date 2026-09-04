package main

import (
	"context"
	"log"
	"time"
)

type Collector struct {
	targetNodes []string
	interval    time.Duration
}

func NewCollector(nodes []string, interval time.Duration) *Collector {
	return &Collector{
		targetNodes: nodes,
		interval:    interval,
	}
}

func (c *Collector) Start(ctx context.Context) {
	log.Printf("[telemetry-agent] Collector initialized for %d targets (interval: %v)", len(c.targetNodes), c.interval)
	// Phase 1 implementation will scrape Prometheus metrics & traces from targets
}
