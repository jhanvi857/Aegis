package recoveryexecutor

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Action struct {
	Type       string
	Parameters map[string]string
}

type Node struct {
	ID        string
	Executor  string
	BaseURL   string
	AuthToken string
	Replicas  int
}

type Result struct {
	StatusCode int
	Request    string
	Response   string
	Latency    time.Duration
}

type RecoveryExecutor interface {
	Execute(ctx context.Context, action Action, node Node) (Result, error)
}

type WebhookExecutor struct {
	Client  *http.Client
	Timeout time.Duration
	Retries int
	Paths   map[string]string
}

type YAMLConfig struct {
	Actions        map[string]string `yaml:"actions"`
	TimeoutSeconds int               `yaml:"timeout_seconds"`
	Retries        int               `yaml:"retries"`
}

func LoadWebhookConfig(data []byte) (*WebhookExecutor, error) {
	cfg := YAMLConfig{}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("invalid recovery executor config: %w", err)
	}
	executor := NewWebhookExecutor(cfg.Actions)
	if cfg.TimeoutSeconds > 0 {
		executor.Timeout = time.Duration(cfg.TimeoutSeconds) * time.Second
		executor.Client = &http.Client{Timeout: executor.Timeout}
	}
	if cfg.Retries >= 0 {
		executor.Retries = cfg.Retries
	}
	return executor, nil
}

func NewWebhookExecutor(paths map[string]string) *WebhookExecutor {
	if len(paths) == 0 {
		paths = map[string]string{
			"RESTART":       "/admin/restart",
			"SCALE_UP":      "/admin/scale",
			"FLUSH_CACHE":   "/admin/flush-cache",
			"INCREASE_POOL": "/admin/pool-size",
		}
	}
	return &WebhookExecutor{Client: &http.Client{Timeout: 5 * time.Second}, Timeout: 5 * time.Second, Retries: 2, Paths: paths}
}

func (e *WebhookExecutor) Execute(ctx context.Context, action Action, node Node) (Result, error) {
	base, err := url.Parse(strings.TrimRight(strings.TrimSpace(node.BaseURL), "/"))
	if err != nil || base.Scheme == "" || base.Host == "" {
		return Result{}, fmt.Errorf("invalid webhook base URL for node %s", node.ID)
	}
	name := strings.ToUpper(strings.TrimSpace(action.Type))
	path, ok := e.Paths[name]
	if !ok {
		return Result{}, fmt.Errorf("unsupported recovery action %q", action.Type)
	}
	base.Path = strings.TrimRight(base.Path, "/") + "/" + strings.TrimLeft(path, "/")
	query := base.Query()
	switch name {
	case "SCALE_UP":
		replicas := node.Replicas + 1
		if raw := action.Parameters["replicas"]; raw != "" {
			if parsed, parseErr := strconv.Atoi(raw); parseErr == nil {
				replicas = parsed
			}
		}
		query.Set("replicas", strconv.Itoa(replicas))
	case "INCREASE_POOL":
		n := action.Parameters["n"]
		if n == "" {
			n = action.Parameters["pool_size"]
		}
		if n == "" {
			return Result{}, errors.New("INCREASE_POOL requires parameter n")
		}
		query.Set("n", n)
	}
	base.RawQuery = query.Encode()
	requestURL := base.String()

	client := e.Client
	if client == nil {
		client = &http.Client{Timeout: e.Timeout}
	}
	retries := e.Retries
	start := time.Now()
	var lastErr error
	for attempt := 0; attempt <= retries; attempt++ {
		req, requestErr := http.NewRequestWithContext(ctx, http.MethodPost, requestURL, nil)
		if requestErr != nil {
			return Result{}, requestErr
		}
		req.Header.Set("X-Admin-Token", node.AuthToken)
		resp, doErr := client.Do(req)
		if doErr != nil {
			lastErr = fmt.Errorf("webhook request failed: %w", doErr)
		} else {
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
			resp.Body.Close()
			result := Result{StatusCode: resp.StatusCode, Request: requestURL, Response: string(body), Latency: time.Since(start)}
			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				return result, nil
			}
			lastErr = fmt.Errorf("webhook returned HTTP %d", resp.StatusCode)
			if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
				return result, lastErr
			}
		}
		if attempt < retries {
			delay := time.Duration(attempt+1) * 100 * time.Millisecond
			select {
			case <-ctx.Done():
				return Result{Request: requestURL, Latency: time.Since(start)}, ctx.Err()
			case <-time.After(delay):
			}
		}
	}
	return Result{Request: requestURL, Latency: time.Since(start)}, lastErr
}
