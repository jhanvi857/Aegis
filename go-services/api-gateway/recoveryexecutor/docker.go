package recoveryexecutor

import (
	"context"
	"fmt"
	"time"

	orchestrator "github.com/aegis/go-services/orchestrator-adapter"
)

// DockerExecutor is an explicit opt-in compatibility executor. It is never
// selected by default and should only be enabled for deployments that
// intentionally grant the gateway access to a local Docker daemon.
type DockerExecutor struct {
	Adapter orchestrator.OrchestratorAdapter
}

func NewDockerExecutor(adapter orchestrator.OrchestratorAdapter) *DockerExecutor {
	return &DockerExecutor{Adapter: adapter}
}

func (e *DockerExecutor) Execute(ctx context.Context, action Action, node Node) (Result, error) {
	if e.Adapter == nil {
		return Result{}, fmt.Errorf("docker executor is not configured")
	}
	start := time.Now()
	var err error
	switch action.Type {
	case "RESTART":
		err = e.Adapter.RestartContainer(ctx, node.ID)
	case "SCALE_UP":
		err = e.Adapter.ScaleService(ctx, node.ID, node.Replicas+1)
	case "FLUSH_CACHE":
		err = e.Adapter.FlushCache(ctx, node.ID)
	case "INCREASE_POOL":
		err = e.Adapter.IncreasePool(ctx, node.ID)
	default:
		err = fmt.Errorf("unsupported recovery action %q", action.Type)
	}
	return Result{Request: "docker://" + node.ID, Latency: time.Since(start)}, err
}
