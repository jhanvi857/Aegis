package orchestrator

import (
	"context"
	"log"
)

type K8sAdapter struct {
	namespace string
}

func NewK8sAdapter(namespace string) *K8sAdapter {
	return &K8sAdapter{
		namespace: namespace,
	}
}

func (k *K8sAdapter) RestartContainer(ctx context.Context, podName string) error {
	log.Printf("[k8s-adapter] Restarting pod %s in namespace %s", podName, k.namespace)
	return nil
}

func (k *K8sAdapter) ScaleService(ctx context.Context, deploymentName string, replicas int) error {
	log.Printf("[k8s-adapter] Scaling deployment %s to %d in namespace %s", deploymentName, replicas, k.namespace)
	return nil
}
