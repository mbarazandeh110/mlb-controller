// internal/adapters/loadbalancer/nginx/nginx.go
package nginx

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"mlb-controller/internal/domain/config"
	"mlb-controller/internal/domain/model"
)

// NginxAdapter implements the LoadBalancerAdapter interface for NGINX.
type NginxAdapter struct {
	clients []*NginxClient
	config  config.LoadBalancerConfig
}

// NewNginxAdapter creates a new NginxAdapter for the given LoadBalancerConfig.
func NewNginxAdapter(cfg config.LoadBalancerConfig) (*NginxAdapter, error) {
	if cfg.GetType() != "nginx" {
		return nil, fmt.Errorf("invalid load balancer type: %s, expected nginx", cfg.GetType())
	}

	var clients []*NginxClient
	for _, addr := range cfg.GetAddresses() {
		client, err := NewNginxClient(addr)
		if err != nil {
			return nil, fmt.Errorf("failed to create client for address %s:%d: %w", addr.IP, addr.Port, err)
		}
		clients = append(clients, client)
	}

	return &NginxAdapter{
		clients: clients,
		config:  cfg,
	}, nil
}

// ListBackends retrieves the current backends for a given upstream from all addresses.
func (a *NginxAdapter) ListBackends(ctx context.Context, upstreamName string) ([]model.Backend, error) {
	var allBackends []model.Backend
	seen := make(map[string]struct{}) // To avoid duplicates

	for _, client := range a.clients {
		path := fmt.Sprintf("dynamic?upstream=%s&verbose=", url.QueryEscape(upstreamName))
		body, err := client.doRequest("GET", path)
		if err != nil {
			return nil, fmt.Errorf("failed to list backends for upstream %s: %w", upstreamName, err)
		}

		// Parse NGINX response (e.g., "server 127.0.0.1:6001 weight=1 max_fails=1 fail_timeout=10;")
		backends, err := parseNginxBackends(string(body))
		if err != nil {
			return nil, fmt.Errorf("failed to parse backends for upstream %s: %w", upstreamName, err)
		}

		// Add unique backends
		for _, b := range backends {
			key := fmt.Sprintf("%s:%d", b.IP, b.Port)
			if _, exists := seen[key]; !exists {
				seen[key] = struct{}{}
				allBackends = append(allBackends, b)
			}
		}
	}

	return allBackends, nil
}

// AddBackend adds a new backend to the specified upstream for all addresses.
func (a *NginxAdapter) AddBackend(ctx context.Context, upstreamName string, backend model.Backend) error {
	for _, client := range a.clients {
		path := fmt.Sprintf("dynamic?upstream=%s&add=&server=%s:%d", url.QueryEscape(upstreamName), url.QueryEscape(backend.IP), backend.Port)
		_, err := client.doRequest("GET", path)
		if err != nil {
			return fmt.Errorf("failed to add backend %s:%d to upstream %s: %w", backend.IP, backend.Port, upstreamName, err)
		}
	}
	return nil
}

// RemoveBackend removes a backend from the specified upstream for all addresses.
func (a *NginxAdapter) RemoveBackend(ctx context.Context, upstreamName string, backend model.Backend) error {
	for _, client := range a.clients {
		path := fmt.Sprintf("dynamic?upstream=%s&remove=&server=%s:%d", url.QueryEscape(upstreamName), url.QueryEscape(backend.IP), backend.Port)
		_, err := client.doRequest("GET", path)
		if err != nil {
			return fmt.Errorf("failed to remove backend %s:%d from upstream %s: %w", backend.IP, backend.Port, upstreamName, err)
		}
	}
	return nil
}

// SyncUpstream ensures the upstream matches the desired state for all addresses (idempotent).
func (a *NginxAdapter) SyncUpstream(ctx context.Context, upstream model.Upstream) error {
	// Get current backends
	currentBackends, err := a.ListBackends(ctx, upstream.Name)
	if err != nil {
		return fmt.Errorf("failed to get current backends for upstream %s: %w", upstream.Name, err)
	}

	// Desired backends from upstream
	desiredBackends := upstream.Backends

	// Find backends to add
	for _, desired := range desiredBackends {
		if !containsBackend(currentBackends, desired) {
			if err := a.AddBackend(ctx, upstream.Name, desired); err != nil {
				return err
			}
		}
	}

	// Find backends to remove
	for _, current := range currentBackends {
		if !containsBackend(desiredBackends, current) {
			if err := a.RemoveBackend(ctx, upstream.Name, current); err != nil {
				return err
			}
		}
	}

	return nil
}

// parseNginxBackends parses NGINX response text into a list of Backends.
func parseNginxBackends(response string) ([]model.Backend, error) {
	var backends []model.Backend
	// Example response: "server 127.0.0.1:6001 weight=1 max_fails=1 fail_timeout=10;"
	lines := strings.Split(response, "\n")
	re := regexp.MustCompile(`server\s+([\d.]+):(\d+)\s+.*;`)

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		matches := re.FindStringSubmatch(line)
		if len(matches) != 3 {
			return nil, fmt.Errorf("invalid backend format: %s", line)
		}

		ip := matches[1]
		portStr := matches[2]
		var port int
		if _, err := fmt.Sscanf(portStr, "%d", &port); err != nil {
			return nil, fmt.Errorf("invalid port in backend %s: %w", line, err)
		}

		backends = append(backends, model.Backend{
			IP:   ip,
			Port: int32(port),
		})
	}

	return backends, nil
}

// containsBackend checks if a backend exists in a list of backends.
func containsBackend(backends []model.Backend, target model.Backend) bool {
	for _, b := range backends {
		if b.IP == target.IP && b.Port == target.Port {
			return true
		}
	}
	return false
}
