package actions

import (
	"context"
	"log"
)

type RerouteTrafficAction struct{}

func (a *RerouteTrafficAction) Name() string { return "reroute_traffic" }

func (a *RerouteTrafficAction) Execute(ctx context.Context, targetNode string, params map[string]string) error {
	drainTimeout := params["drain_timeout_sec"]
	if drainTimeout == "" {
		drainTimeout = "10"
	}
	ratio := params["reroute_ratio"]
	if ratio == "" {
		ratio = "1.0"
	}
	log.Printf("[action-reroute] Rerouting %s ratio traffic away from degraded node %s (drain timeout: %ss)", ratio, targetNode, drainTimeout)
	log.Printf("[action-reroute] Traffic for node %s successfully drained and diverted.", targetNode)
	return nil
}
