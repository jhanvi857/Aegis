package actions

import (
	"context"
	"fmt"
	"log"

	orchestrator "github.com/aegis/go-services/orchestrator-adapter"
)

type RerouteTrafficAction struct {
	Adapter orchestrator.OrchestratorAdapter
}

func NewRerouteTrafficAction(adapter orchestrator.OrchestratorAdapter) *RerouteTrafficAction {
	return &RerouteTrafficAction{Adapter: adapter}
}

func (a *RerouteTrafficAction) Name() string { return "reroute_traffic" }

func (a *RerouteTrafficAction) Execute(ctx context.Context, targetNode string, params map[string]string) error {
	if a.Adapter == nil {
		return fmt.Errorf("reroute_traffic is unsupported: no traffic-management adapter is configured for %q", targetNode)
	}
	targetRoute := ""
	if params != nil {
		targetRoute = params["target_route"]
	}
	if targetRoute == "" {
		targetRoute = "backup"
	}
	log.Printf("[action-reroute] Initiating traffic reroute for node %s to %s", targetNode, targetRoute)
	if err := a.Adapter.RerouteTraffic(ctx, targetNode, targetRoute); err != nil {
		log.Printf("[action-reroute] Adapter returned error for %s: %v", targetNode, err)
		return err
	}
	log.Printf("[action-reroute] Node %s traffic rerouted successfully.", targetNode)
	return nil
}
