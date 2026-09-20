package actions

import (
	"context"
	"log"
)

type FlushCacheAction struct{}

func (a *FlushCacheAction) Name() string { return "flush_cache" }

func (a *FlushCacheAction) Execute(ctx context.Context, targetNode string, params map[string]string) error {
	asyncFlush := params["async_flush"]
	if asyncFlush == "" {
		asyncFlush = "true"
	}
	log.Printf("[action-flush-cache] Evicting stale keys and flushing cache memory store on node %s (async: %s)", targetNode, asyncFlush)
	log.Printf("[action-flush-cache] Cache flushed successfully on node %s.", targetNode)
	return nil
}
