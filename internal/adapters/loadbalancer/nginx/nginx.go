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
		allBackends = append(allBackends, backends...)
	}

	return allBackends, nil
}

// AddBackend adds a new backend to the specified upstream for all addresses or updates its weight.
func (a *NginxAdapter) AddBackend(ctx context.Context, upstreamName string, backend model.Backend) error {
	// Check current backends to determine if we need to update weight
	currentBackends, err := a.ListBackends(ctx, upstreamName)
	if err != nil {
		return fmt.Errorf("failed to check current backends for upstream %s: %w", upstreamName, err)
	}

	backend.Weight = -1
	for _, b := range currentBackends {
		if b.IP == backend.IP && b.Port == backend.Port {
			backend.Weight = b.Weight + 1
			break
		}
	}

	// Send weight update request to all addresses
	for _, client := range a.clients {
		path := ""
		if backend.Weight <= 0 {
			path = fmt.Sprintf("dynamic?upstream=%s&add=&server=%s:%d&weight=1", url.QueryEscape(upstreamName), url.QueryEscape(backend.IP), backend.Port)
		} else {
			if backend.Weight <= 1 {
				backend.Weight = 1
			}
			path = fmt.Sprintf("dynamic?upstream=%s&server=%s:%d&weight=%d", url.QueryEscape(upstreamName), url.QueryEscape(backend.IP), backend.Port, backend.Weight)
		}
		_, err := client.doRequest("GET", path)
		if err != nil {
			return fmt.Errorf("failed to update backend %s:%d (weight=%d) to upstream %s: %w", backend.IP, backend.Port, backend.Weight, upstreamName, err)
		}
	}
	return nil
}

// RemoveBackend removes a backend from the specified upstream for all addresses or decreases its weight.
func (a *NginxAdapter) RemoveBackend(ctx context.Context, upstreamName string, backend model.Backend) error {
	// Check current backends to determine weight
	currentBackends, err := a.ListBackends(ctx, upstreamName)
	if err != nil {
		return fmt.Errorf("failed to check current backends for upstream %s: %w", upstreamName, err)
	}

	backend.Weight = -1
	for _, b := range currentBackends {
		if b.IP == backend.IP && b.Port == backend.Port {
			backend.Weight = b.Weight
			break
		}
	}

	if backend.Weight == -1 {
		return fmt.Errorf("backend %s:%d not found in upstream %s", backend.IP, backend.Port, upstreamName)
	}

	for _, client := range a.clients {
		path := ""
		if backend.Weight <= 1 {
			// Remove backend completely
			path = fmt.Sprintf("dynamic?upstream=%s&remove=&server=%s:%d", url.QueryEscape(upstreamName), url.QueryEscape(backend.IP), backend.Port)
		} else {
			// Decrease weight by 1
			backend.Weight = backend.Weight - 1
			path = fmt.Sprintf("dynamic?upstream=%s&server=%s:%d&weight=%d", url.QueryEscape(upstreamName), url.QueryEscape(backend.IP), backend.Port, backend.Weight)
		}
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

	// Calculate desired weights by counting duplicates
	desiredWeights := make(map[string]int)
	for _, b := range upstream.Backends {
		key := fmt.Sprintf("%s:%d", b.IP, b.Port)
		desiredWeights[key]++
	}

	// Convert current backends to map for comparison
	currentWeights := make(map[string]int)
	for _, b := range currentBackends {
		key := fmt.Sprintf("%s:%d", b.IP, b.Port)
		currentWeights[key] = b.Weight
	}

	// Sync backends: add or update weights
	for key, desiredWeight := range desiredWeights {
		parts := strings.Split(key, ":")
		ip := parts[0]
		var port int
		fmt.Sscanf(parts[1], "%d", &port)

		currentWeight, exists := currentWeights[key]

		if !exists || currentWeight != desiredWeight {
			for _, client := range a.clients {
				path := fmt.Sprintf("dynamic?upstream=%s&server=%s:%d&weight=%d", url.QueryEscape(upstream.Name), url.QueryEscape(ip), port, desiredWeight)
				_, err := client.doRequest("GET", path)
				if err != nil {
					return fmt.Errorf("failed to update backend %s:%d (weight=%d) in upstream %s: %w", ip, port, desiredWeight, upstream.Name, err)
				}
			}
		}
	}

	// Remove or decrease weights for backends that shouldn't exist
	for key, currentWeight := range currentWeights {
		parts := strings.Split(key, ":")
		ip := parts[0]
		var port int
		fmt.Sscanf(parts[1], "%d", &port)

		desiredWeight, exists := desiredWeights[key]

		if !exists {
			// Remove backend completely
			for _, client := range a.clients {
				path := fmt.Sprintf("dynamic?upstream=%s&remove=&server=%s:%d", url.QueryEscape(upstream.Name), url.QueryEscape(ip), port)
				_, err := client.doRequest("GET", path)
				if err != nil {
					return fmt.Errorf("failed to remove backend %s:%d from upstream %s: %w", ip, port, upstream.Name, err)
				}
			}
		} else if currentWeight > desiredWeight {
			// Decrease weight
			for _, client := range a.clients {
				path := fmt.Sprintf("dynamic?upstream=%s&server=%s:%d&weight=%d", url.QueryEscape(upstream.Name), url.QueryEscape(ip), port, desiredWeight)
				_, err := client.doRequest("GET", path)
				if err != nil {
					return fmt.Errorf("failed to update backend %s:%d (weight=%d) in upstream %s: %w", ip, port, desiredWeight, upstream.Name, err)
				}
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
	re := regexp.MustCompile(`server\s+([\d.]+):(\d+)\s+weight=(\d+).*;`)

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		matches := re.FindStringSubmatch(line)
		if len(matches) != 4 {
			return nil, fmt.Errorf("invalid backend format: %s", line)
		}

		ip := matches[1]
		portStr := matches[2]
		weightStr := matches[3]
		var port, weight int
		if _, err := fmt.Sscanf(portStr, "%d", &port); err != nil {
			return nil, fmt.Errorf("invalid port in backend %s: %w", line, err)
		}
		if _, err := fmt.Sscanf(weightStr, "%d", &weight); err != nil {
			return nil, fmt.Errorf("invalid weight in backend %s: %w", line, err)
		}

		backends = append(backends, model.Backend{
			IP:     ip,
			Port:   int32(port),
			Weight: weight,
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
