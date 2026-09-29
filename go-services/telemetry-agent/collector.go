package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	telemetrypb "github.com/aegis/go-services/shared/pb/telemetry"
	"google.golang.org/protobuf/proto"
)

type TargetTelemetry struct {
	Status        string  `json:"status"`
	NodeID        string  `json:"node_id"`
	CPU           float64 `json:"cpu"`
	Memory        float64 `json:"memory"`
	LatencyMs     float64 `json:"latency_ms"`
	ErrorRate     float64 `json:"error_rate"`
	ActiveFault   string  `json:"active_fault"`
	RemainingSec  int     `json:"remaining_sec"`
}

type Collector struct {
	targetNodes []string
	interval    time.Duration
	publisher   *KafkaPublisher
	httpClient  *http.Client
}

func NewCollector(nodes []string, interval time.Duration, publisher *KafkaPublisher) *Collector {
	return &Collector{
		targetNodes: nodes,
		interval:    interval,
		publisher:   publisher,
		httpClient:  &http.Client{Timeout: 1500 * time.Millisecond},
	}
}

func (c *Collector) Start(ctx context.Context) {
	log.Printf("[telemetry-agent] Collector initialized for %d targets (interval: %v)", len(c.targetNodes), c.interval)

	go func() {
		ticker := time.NewTicker(c.interval)
		defer ticker.Stop()

		portMap := map[string]int{
			"gateway": 8080,
			"node-a":  8081,
			"node-b":  8082,
			"node-c":  8083,
			"node-d":  8084,
		}

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				nowNs := time.Now().UnixNano()
				batch := &telemetrypb.TelemetryBatch{
					Metrics: make([]*telemetrypb.MetricPoint, 0),
					Logs:    make([]*telemetrypb.LogEntry, 0),
				}

				for _, node := range c.targetNodes {
					port, ok := portMap[node]
					if !ok {
						port = 8081
					}

					urls := []string{
						fmt.Sprintf("http://%s:%d/health", node, port),
						fmt.Sprintf("http://localhost:%d/health", port),
					}

					var telem TargetTelemetry
					collected := false

					for _, u := range urls {
						resp, err := c.httpClient.Get(u)
						if err == nil {
							if json.NewDecoder(resp.Body).Decode(&telem) == nil {
								collected = true
							}
							_ = resp.Body.Close()
							break
						}
					}

					if collected {
						batch.Metrics = append(batch.Metrics,
							&telemetrypb.MetricPoint{TimestampUnixNano: nowNs, ServiceId: node, MetricName: "cpu_usage", Value: telem.CPU},
							&telemetrypb.MetricPoint{TimestampUnixNano: nowNs, ServiceId: node, MetricName: "memory_usage", Value: telem.Memory},
							&telemetrypb.MetricPoint{TimestampUnixNano: nowNs, ServiceId: node, MetricName: "p95_latency_ms", Value: telem.LatencyMs},
							&telemetrypb.MetricPoint{TimestampUnixNano: nowNs, ServiceId: node, MetricName: "error_rate", Value: telem.ErrorRate},
						)
						if telem.ActiveFault != "" {
							batch.Logs = append(batch.Logs, &telemetrypb.LogEntry{
								TimestampUnixNano: nowNs,
								ServiceId:         node,
								Level:             "WARN",
								Message:           fmt.Sprintf("Telemetry agent detected active fault: %s on %s", telem.ActiveFault, node),
							})
						}
					}
				}

				if len(batch.Metrics) > 0 && c.publisher != nil {
					data, err := proto.Marshal(batch)
					if err == nil {
						_ = c.publisher.Publish(data)
					}
				}
			}
		}
	}()
}
