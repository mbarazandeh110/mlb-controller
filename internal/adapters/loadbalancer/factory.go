// internal/adapters/loadbalancer/factory.go
package loadbalancer

import (
	"fmt"

	"mlb-controller/internal/adapters/loadbalancer/nginx"
	"mlb-controller/internal/domain/config"
	"mlb-controller/internal/ports/loadbalancer"
	"mlb-controller/internal/ports/logging"
)

// Factory defines an interface for creating LoadBalancerAdapter instances.
type Factory interface {
	CreateLoadBalancerAdapter(lbConfig config.LoadBalancerConfig, logger logging.Logger) (loadbalancer.LoadBalancerAdapter, error)
}

// LoadBalancerFactory implements the Factory interface.
type LoadBalancerFactory struct{}

// NewLoadBalancerFactory creates a new LoadBalancerFactory instance.
func NewLoadBalancerFactory() Factory {
	return &LoadBalancerFactory{}
}

// CreateLoadBalancerAdapter creates a LoadBalancerAdapter based on the load balancer type.
func (f *LoadBalancerFactory) CreateLoadBalancerAdapter(lbConfig config.LoadBalancerConfig, logger logging.Logger) (loadbalancer.LoadBalancerAdapter, error) {
	switch lbConfig.GetType() {
	case "nginx":
		nginxCfg, ok := lbConfig.(config.NginxConfig)
		if !ok {
			return nil, fmt.Errorf("failed to cast load balancer config to NginxConfig for %s", lbConfig.GetName())
		}
		return nginx.NewNginxAdapter(nginxCfg, logger)
	case "envoy":
		// Placeholder for Envoy; implement when needed
		return nil, fmt.Errorf("envoy adapter not implemented")
	default:
		return nil, fmt.Errorf("unsupported load balancer type: %s", lbConfig.GetType())
	}
}
