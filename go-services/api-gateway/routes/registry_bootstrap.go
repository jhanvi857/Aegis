package routes

import (
	"context"

	"github.com/aegis/go-services/api-gateway/storage"
)

// BootstrapTopologyRegistry makes the active YAML topology available to the
// registry-backed graph builder on a fresh installation. Existing registry
// data is authoritative and is never overwritten.
func BootstrapTopologyRegistry(ctx context.Context, store storage.Store) error {
	services, err := store.ListServices(ctx, "", "")
	if err != nil {
		return err
	}
	if len(services) > 0 {
		return nil
	}

	State.mu.RLock()
	defer State.mu.RUnlock()

	for _, id := range State.ServiceOrder {
		svc := State.Services[id]
		if svc == nil {
			continue
		}
		if err := store.UpsertService(ctx, &storage.TopologyService{
			ID:          svc.ID,
			Name:        svc.Name,
			Type:        svc.Type,
			Replicas:    svc.Replicas,
			MaxReplicas: svc.MaxReplicas,
			Status:      "unconnected",
		}); err != nil {
			return err
		}
	}

	for _, id := range State.ServiceOrder {
		svc := State.Services[id]
		if svc == nil {
			continue
		}
		for _, dependencyID := range svc.Dependencies {
			if err := store.UpsertEdge(ctx, &storage.TopologyEdge{
				SourceServiceID: id,
				TargetServiceID: dependencyID,
				Relationship:    "dependency",
				Source:          "manual",
				State:           "active",
				EdgeType:        "sync",
			}); err != nil {
				return err
			}
		}
	}

	return nil
}

func ReplaceTopologyRegistry(ctx context.Context, store storage.Store) error {
	services, err := store.ListServices(ctx, "", "")
	if err != nil {
		return err
	}
	for _, service := range services {
		if err := store.DeleteService(ctx, service.ID); err != nil {
			return err
		}
	}
	return BootstrapTopologyRegistry(ctx, store)
}
