package main

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/aegis/go-services/chaos-engine/injectors"
)

type ActiveEpisode struct {
	ID         string            `json:"id"`
	FaultType  string            `json:"fault_type"`
	TargetNode string            `json:"target_node"`
	StartedAt  string            `json:"started_at"`
	DurationSec int              `json:"duration_sec"`
	Parameters map[string]string `json:"parameters"`
	cancel     context.CancelFunc `json:"-"`
}

type ChaosScheduler struct {
	injectors map[string]injectors.Injector
	logger    *EpisodeLogger
	active    map[string]*ActiveEpisode
	mu        sync.RWMutex
}

func NewChaosScheduler(logger *EpisodeLogger) *ChaosScheduler {
	s := &ChaosScheduler{
		injectors: make(map[string]injectors.Injector),
		logger:    logger,
		active:    make(map[string]*ActiveEpisode),
	}
	s.registerDefaultInjectors()
	return s
}

func (s *ChaosScheduler) registerDefaultInjectors() {
	s.injectors["latency"] = &injectors.LatencyInjector{}
	s.injectors["kill_service"] = &injectors.KillServiceInjector{}
	s.injectors["cpu_stress"] = &injectors.CPUStressInjector{}
	s.injectors["memory_leak"] = &injectors.MemoryLeakInjector{}
	s.injectors["packet_loss"] = &injectors.PacketLossInjector{}
	s.injectors["mq_lag"] = &injectors.MQLagInjector{}
	s.injectors["cache_down"] = &injectors.CacheDownInjector{}
	s.injectors["db_lock"] = &injectors.DBLockInjector{}
	s.injectors["slow_query"] = &injectors.SlowQueryInjector{}
	s.injectors["thread_exhaustion"] = &injectors.ThreadExhaustionInjector{}
}

func (s *ChaosScheduler) RunEpisode(parentCtx context.Context, faultType string, targetNode string, duration time.Duration, params map[string]string) error {
	inj, exists := s.injectors[faultType]
	if !exists {
		log.Printf("[chaos-scheduler] Unknown injector type: %s", faultType)
		return fmt.Errorf("unknown injector type: %s", faultType)
	}

	episodeID := s.logger.StartEpisode(faultType, targetNode, params)
	ctx, cancel := context.WithTimeout(parentCtx, duration)

	ep := &ActiveEpisode{
		ID:          episodeID,
		FaultType:   faultType,
		TargetNode:  targetNode,
		StartedAt:   time.Now().Format("15:04:05"),
		DurationSec: int(duration.Seconds()),
		Parameters:  params,
		cancel:      cancel,
	}

	s.mu.Lock()
	s.active[targetNode] = ep
	s.mu.Unlock()

	// Apply physical injection
	err := inj.Inject(ctx, targetNode, duration, params)
	if err != nil {
		log.Printf("[chaos-scheduler] Episode %s injection error: %v", episodeID, err)
	}

	// Wait for duration or cancellation
	<-ctx.Done()

	// Revert physical injection
	revertCtx, revertCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer revertCancel()
	_ = inj.Revert(revertCtx, targetNode)

	s.mu.Lock()
	if current, ok := s.active[targetNode]; ok && current.ID == episodeID {
		delete(s.active, targetNode)
	}
	s.mu.Unlock()

	s.logger.EndEpisode(episodeID, err == nil)
	return err
}

func (s *ChaosScheduler) StopEpisode(targetNode string) error {
	s.mu.Lock()
	ep, exists := s.active[targetNode]
	if !exists {
		// Also try finding by episode ID
		for node, e := range s.active {
			if e.ID == targetNode {
				ep = e
				targetNode = node
				exists = true
				break
			}
		}
	}
	if exists && ep != nil && ep.cancel != nil {
		ep.cancel()
		delete(s.active, targetNode)
	}
	s.mu.Unlock()

	// Direct physical revert
	revertCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	for _, inj := range s.injectors {
		_ = inj.Revert(revertCtx, targetNode)
	}
	return nil
}

func (s *ChaosScheduler) GetActiveEpisodes() []*ActiveEpisode {
	s.mu.RLock()
	defer s.mu.RUnlock()

	res := make([]*ActiveEpisode, 0, len(s.active))
	for _, ep := range s.active {
		res = append(res, ep)
	}
	return res
}
