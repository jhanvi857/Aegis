package actions

import (
	"context"
	"fmt"
)

type RerouteTrafficAction struct{}

func (a *RerouteTrafficAction) Name() string { return "reroute_traffic" }

func (a *RerouteTrafficAction) Execute(ctx context.Context, targetNode string, params map[string]string) error {
	return fmt.Errorf("reroute_traffic action not implemented (Phase 4)")
}
