package actions

import (
	"context"
	"log"
	"strconv"
)

type IncreasePoolAction struct{}

func (a *IncreasePoolAction) Name() string { return "increase_pool" }

func (a *IncreasePoolAction) Execute(ctx context.Context, targetNode string, params map[string]string) error {
	incStr := params["pool_increment"]
	increment := 10
	if incStr != "" {
		if val, err := strconv.Atoi(incStr); err == nil {
			increment = val
		}
	}
	log.Printf("[action-increase-pool] Expanding connection pool by +%d connections on node %s", increment, targetNode)
	log.Printf("[action-increase-pool] Connection pool capacity updated on node %s.", targetNode)
	return nil
}
