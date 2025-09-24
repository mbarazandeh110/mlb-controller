package kubernetes

import (
	"context"

	"mlb-controller/internal/domain/model"
)

// KubernetesPort defines the interface for interacting with Kubernetes resources.
type KubernetesPort interface {
	// WatchServices watches Kubernetes Services with specific labels and sends events to the provided channel.
	WatchServices(ctx context.Context, labels map[string]string, events chan<- ServiceEvent) error

	// WatchPods watches Kubernetes Pods and sends events to the provided channel.
	WatchPods(ctx context.Context, events chan<- PodEvent) error

	// WatchNodes watches Kubernetes Nodes and sends events to the provided channel.
	WatchNodes(ctx context.Context, events chan<- NodeEvent) error

	// GetPodsForSelector retrieves Pods matching the given label selector in a namespace.
	GetPodsForSelector(ctx context.Context, namespace string, selector map[string]string) ([]model.Pod, error)

	// GetNodeByName retrieves a Node by its name.
	GetNodeByName(ctx context.Context, name string) (model.Node, error)
}

// ServiceEvent represents an event related to a Kubernetes Service.
type ServiceEvent struct {
	Type    EventType // Add, Update, Delete
	Service model.Service
}

// PodEvent represents an event related to a Kubernetes Pod.
type PodEvent struct {
	Type EventType // Add, Update, Delete
	Pod  model.Pod
}

// NodeEvent represents an event related to a Kubernetes Node.
type NodeEvent struct {
	Type EventType // Add, Update, Delete
	Node model.Node
}

// EventType defines the type of Kubernetes resource event.
type EventType string

const (
	EventTypeAdd    EventType = "Add"
	EventTypeUpdate EventType = "Update"
	EventTypeDelete EventType = "Delete"
)
