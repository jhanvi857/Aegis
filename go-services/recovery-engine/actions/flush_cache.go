package actions

import (
	"context"
	"fmt"
	"log"

	orchestrator "github.com/aegis/go-services/orchestrator-adapter"
)

type FlushCacheAction struct {
	Adapter orchestrator.OrchestratorAdapter
}

func NewFlushCacheAction(adapter orchestrator.OrchestratorAdapter) *FlushCacheAction {
	return &FlushCacheAction{Adapter: adapter}
}

func (a *FlushCacheAction) Name() string { return "flush_cache" }

func (a *FlushCacheAction) Execute(ctx context.Context, targetNode string, params map[string]string) error {
	if a.Adapter == nil {
		return fmt.Errorf("flush_cache is unsupported: no cache-management adapter is configured for %q", targetNode)
	}
	log.Printf("[action-flush_cache] Initiating cache flush for node %s", targetNode)
	if err := a.Adapter.FlushCache(ctx, targetNode); err != nil {
		log.Printf("[action-flush_cache] Adapter returned error for %s: %v", targetNode, err)
		return err
	}
	log.Printf("[action-flush_cache] Node %s cache flushed successfully.", targetNode)
	return nil
}
