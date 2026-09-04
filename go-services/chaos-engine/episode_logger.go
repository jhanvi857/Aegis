package main

import (
	"fmt"
	"log"
	"sync"
	"time"
)

type EpisodeRecord struct {
	ID         string            `json:"id"`
	FaultType  string            `json:"fault_type"`
	TargetNode string            `json:"target_node"`
	Parameters map[string]string `json:"parameters"`
	StartTime  time.Time         `json:"start_time"`
	EndTime    time.Time         `json:"end_time,omitempty"`
	Success    bool              `json:"success"`
}

type EpisodeLogger struct {
	mu       sync.Mutex
	episodes map[string]*EpisodeRecord
}

func NewEpisodeLogger() *EpisodeLogger {
	return &EpisodeLogger{
		episodes: make(map[string]*EpisodeRecord),
	}
}

func (l *EpisodeLogger) StartEpisode(faultType string, targetNode string, params map[string]string) string {
	l.mu.Lock()
	defer l.mu.Unlock()

	id := fmt.Sprintf("ep-%d", time.Now().UnixNano())
	record := &EpisodeRecord{
		ID:         id,
		FaultType:  faultType,
		TargetNode: targetNode,
		Parameters: params,
		StartTime:  time.Now(),
	}
	l.episodes[id] = record
	log.Printf("[episode-logger] Started episode %s (fault: %s, target: %s)", id, faultType, targetNode)
	return id
}

func (l *EpisodeLogger) EndEpisode(id string, success bool) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if record, exists := l.episodes[id]; exists {
		record.EndTime = time.Now()
		record.Success = success
		log.Printf("[episode-logger] Ended episode %s (duration: %v, success: %v)", id, record.EndTime.Sub(record.StartTime), success)
	}
}
