package kubernetes

import (
	"context"
	"mlb-controller/internal/domain/model"
)

// KubernetesAdapter defines the interface for interacting with the Kubernetes API.
// This acts as a Port in the Hexagonal Architecture.
type KubernetesAdapter interface {
	// StartInformer starts the informers for services, pods, and nodes.
	StartInformer(ctx context.Context) error
	// GetServices returns a list of all services in the cluster.
	GetServices() ([]model.Service, error)
	// GetPodsBySelector returns a list of pods matching the given label selector.
	GetPodsBySelector(namespace string, selector map[string]string) ([]model.Pod, error)
	// GetNode returns a node by its name.
	GetNode(name string) (model.Node, error)
	// WaitForCacheSync waits for the informer caches to be synced.
	WaitForCacheSync(ctx context.Context) bool
}
