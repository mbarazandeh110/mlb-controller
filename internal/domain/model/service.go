// internal/domain/model/service.go
package model

// Service represents a Kubernetes Service with metadata relevant for load balancer upstreams.
type Service struct {
	// Name is the name of the service.
	Name string
	// Namespace is the namespace of the service.
	Namespace string
	// Labels contains the service labels (e.g., mlb-loadbalancer, mlb-upstream, mlb-port).
	Labels map[string]string
	// Selector contains the labels used to select associated pods.
	Selector map[string]string
	// ServiceType is the type of service (e.g., LoadBalancer, NodePort).
	ServiceType string
	// NodePort is the node port assigned to the service (if applicable).
	NodePort int32
}
