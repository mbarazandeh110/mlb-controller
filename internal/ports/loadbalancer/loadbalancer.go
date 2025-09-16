// internal/ports/loadbalancer/loadbalancer.go
package loadbalancer

import (
	"context"
	"mlb-controller/internal/domain/model"
)

// LoadBalancerAdapter defines the interface for interacting with a load balancer (e.g., NGINX, Envoy).
type LoadBalancerAdapter interface {
	// SyncUpstream ensures the upstream matches the desired state (idempotent).
	SyncUpstream(ctx context.Context, upstream model.Upstream) error
}
