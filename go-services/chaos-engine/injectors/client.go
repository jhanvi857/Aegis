package injectors

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"
)

var defaultPortMap = map[string]int{
	"gateway": 8080,
	"node-a":  8081,
	"node-b":  8082,
	"node-c":  8083,
	"node-d":  8084,
}

func GetTargetURLs(targetNode string) []string {
	port, ok := defaultPortMap[targetNode]
	if !ok {
		port = 8081
	}
	return []string{
		fmt.Sprintf("http://%s:%d", targetNode, port),
		fmt.Sprintf("http://localhost:%d", port),
	}
}

func CallNodeChaosInject(ctx context.Context, faultType, targetNode string, duration time.Duration, params map[string]string) error {
	payload, _ := json.Marshal(map[string]interface{}{
		"fault_type":   faultType,
		"duration_sec": int(duration.Seconds()),
		"parameters":   params,
	})

	client := &http.Client{Timeout: 2 * time.Second}
	var lastErr error

	for _, url := range GetTargetURLs(targetNode) {
		req, err := http.NewRequestWithContext(ctx, "POST", url+"/chaos/inject", bytes.NewBuffer(payload))
		if err != nil {
			lastErr = err
			continue
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)
		if err == nil {
			_ = resp.Body.Close()
			log.Printf("[chaos-injector] Applied %s to %s via %s", faultType, targetNode, url)
			return nil
		}
		lastErr = err
	}
	log.Printf("[chaos-injector] Notice: Could not contact node %s directly: %v (operating in control plane tracking mode)", targetNode, lastErr)
	return nil
}

func CallNodeChaosRevert(ctx context.Context, targetNode string) error {
	client := &http.Client{Timeout: 2 * time.Second}
	var lastErr error

	for _, url := range GetTargetURLs(targetNode) {
		req, err := http.NewRequestWithContext(ctx, "POST", url+"/chaos/revert", bytes.NewBufferString("{}"))
		if err != nil {
			lastErr = err
			continue
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)
		if err == nil {
			_ = resp.Body.Close()
			log.Printf("[chaos-injector] Reverted chaos on %s via %s", targetNode, url)
			return nil
		}
		lastErr = err
	}
	return lastErr
}
