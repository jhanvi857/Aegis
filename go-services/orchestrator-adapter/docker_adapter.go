package orchestrator

import (
	"context"
	"log"
)

type OrchestratorAdapter interface {
	RestartContainer(ctx context.Context, containerName string) error
	ScaleService(ctx context.Context, serviceName string, replicas int) error
}

type DockerAdapter struct{}

func NewDockerAdapter() *DockerAdapter {
	return &DockerAdapter{}
}

func (d *DockerAdapter) RestartContainer(ctx context.Context, containerName string) error {
	log.Printf("[docker-adapter] Restarting container: %s", containerName)
	return nil
}

func (d *DockerAdapter) ScaleService(ctx context.Context, serviceName string, replicas int) error {
	log.Printf("[docker-adapter] Scaling service %s to %d replicas", serviceName, replicas)
	return nil
}
