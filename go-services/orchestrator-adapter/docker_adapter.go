package orchestrator

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os/exec"
	"strings"
	"time"
)

type OrchestratorAdapter interface {
	RestartContainer(ctx context.Context, containerName string) error
	ScaleService(ctx context.Context, serviceName string, replicas int) error
}

type DockerAdapter struct{}

func NewDockerAdapter() *DockerAdapter {
	return &DockerAdapter{}
}

// resolveContainerName finds the real running Docker container corresponding to a service ID
func (d *DockerAdapter) resolveContainerName(ctx context.Context, target string) string {
	target = strings.TrimSpace(target)
	if target == "" {
		return ""
	}

	// 1. Direct inspect check
	cmd := exec.CommandContext(ctx, "docker", "inspect", "--format", "{{.Name}}", target)
	if out, err := cmd.Output(); err == nil {
		clean := strings.Trim(strings.TrimSpace(string(out)), "/")
		if clean != "" {
			return clean
		}
	}

	// 2. Discover via docker ps names
	psCmd := exec.CommandContext(ctx, "docker", "ps", "--format", "{{.Names}}")
	out, err := psCmd.Output()
	if err == nil {
		lines := strings.Split(string(out), "\n")
		// Exact or contains match
		for _, line := range lines {
			cName := strings.TrimSpace(line)
			if cName == "" {
				continue
			}
			lowerC := strings.ToLower(cName)
			lowerT := strings.ToLower(target)
			if strings.EqualFold(cName, target) || strings.HasSuffix(lowerC, "-"+lowerT) || strings.HasPrefix(lowerC, lowerT+"-") {
				return cName
			}
		}
		for _, line := range lines {
			cName := strings.TrimSpace(line)
			if cName != "" && strings.Contains(strings.ToLower(cName), strings.ToLower(target)) {
				return cName
			}
		}
	}

	return target
}

func (d *DockerAdapter) RestartContainer(ctx context.Context, containerName string) error {
	resolved := d.resolveContainerName(ctx, containerName)
	if resolved == "" {
		resolved = containerName
	}

	log.Printf("[docker-adapter] Executing real restart for container: %s (target: %s)", resolved, containerName)

	cmdCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, "docker", "restart", resolved)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg == "" {
			errMsg = err.Error()
		}
		log.Printf("[docker-adapter] Error restarting container %s: %s", resolved, errMsg)
		return fmt.Errorf("docker restart failed for %s: %s", resolved, errMsg)
	}

	log.Printf("[docker-adapter] Successfully restarted container %s: %s", resolved, strings.TrimSpace(stdout.String()))
	return nil
}

func (d *DockerAdapter) ScaleService(ctx context.Context, serviceName string, replicas int) error {
	log.Printf("[docker-adapter] Scaling service %s to %d replicas", serviceName, replicas)

	cmdCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	scaleArg := fmt.Sprintf("%s=%d", serviceName, replicas)
	cmd := exec.CommandContext(cmdCtx, "docker", "compose", "up", "-d", "--scale", scaleArg, "--no-recreate")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg == "" {
			errMsg = err.Error()
		}
		return fmt.Errorf("docker compose scale failed for %s: %s", serviceName, errMsg)
	}
	log.Printf("[docker-adapter] Scale output: %s", strings.TrimSpace(stdout.String()))
	return nil
}
