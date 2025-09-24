// internal/domain/model/pod.go
package model

// Pod represents a Kubernetes Pod with details relevant for load balancer upstreams.
type Pod struct {
	// Name is the name of the pod.
	Name string
	// Namespace is the namespace of the pod.
	Namespace string
	// NodeName is the name of the node where the pod is running.
	NodeName string
	// IP is the pod's IP address.
	IP string
	// Status is the pod's status (e.g., Running, Terminating).
	Status string
	// Ready is the pod's readiness status.
	Ready  bool
	Labels map[string]string
}
