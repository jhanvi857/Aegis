package main

import (
	"context"
	"log"
	"time"

	"github.com/aegis/go-services/chaos-engine/injectors"
)

type ChaosScheduler struct {
	injectors map[string]injectors.Injector
	logger    *EpisodeLogger
}

func NewChaosScheduler(logger *EpisodeLogger) *ChaosScheduler {
	s := &ChaosScheduler{
		injectors: make(map[string]injectors.Injector),
		logger:    logger,
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

func (s *ChaosScheduler) RunEpisode(ctx context.Context, faultType string, targetNode string, duration time.Duration, params map[string]string) error {
	inj, exists := s.injectors[faultType]
	if !exists {
		log.Printf("[chaos-scheduler] Unknown injector type: %s", faultType)
		return nil
	}

	episodeID := s.logger.StartEpisode(faultType, targetNode, params)
	err := inj.Inject(ctx, targetNode, duration, params)
	s.logger.EndEpisode(episodeID, err == nil)
	return err
}
