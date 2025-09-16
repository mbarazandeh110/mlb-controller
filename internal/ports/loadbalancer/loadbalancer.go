// internal/ports/loadbalancer/loadbalancer.go
package loadbalancer

import (
	"context"
	"mlb-controller/internal/domain/model"
)

// LoadBalancerAdapter defines the interface for interacting with a load balancer (e.g., NGINX, Envoy).
type LoadBalancerAdapter interface {
	// ListBackends retrieves the current backends for a given upstream.
	ListBackends(ctx context.Context, upstreamName string) (map[string][]model.Backend, error)
	// AddBackend adds a new backend to the specified upstream.
	AddBackend(ctx context.Context, upstreamName string, backend model.Backend) error
	// RemoveBackend removes a backend from the specified upstream.
	RemoveBackend(ctx context.Context, upstreamName string, backend model.Backend) error
	// SyncUpstream ensures the upstream matches the desired state (idempotent).
	SyncUpstream(ctx context.Context, upstream model.Upstream) error
}
