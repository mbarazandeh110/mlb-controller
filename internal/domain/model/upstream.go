// internal/domain/model/upstream.go
package model

// Upstream represents a load balancer upstream configuration.
type Upstream struct {
	// Name is the upstream name (from mlb-upstream label).
	Name string
	// LoadBalancer is the load balancer name (from mlb-loadbalancer label).
	LoadBalancer string
	// Backends is the list of backend servers for this upstream.
	Backends []Backend
	// Type is the load balancer type (e.g., nginx, envoy).
	Type string
	// Config is the load balancer configuration (e.g., NginxConfig or EnvoyConfig).
	Config interface{}
	// PreserveExistingBackends indicates that existing backends on the external LB should be left untouched.
	// Used by the Safety Net (allow_empty_upstream: false) and Fail-Static error handling to prevent accidental draining.
	PreserveExistingBackends bool
}
