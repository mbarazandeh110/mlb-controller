package nginx

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"mlb-controller/internal/domain/config"
	"mlb-controller/internal/domain/model"

	"github.com/hashicorp/go-multierror"
	"golang.org/x/sync/semaphore"
)

// NginxAdapter implements the LoadBalancerAdapter interface for NGINX.
type NginxAdapter struct {
	clients   []*NginxClient
	config    config.LoadBalancerConfig
	semaphore *semaphore.Weighted // Added for concurrency control
}

// NewNginxAdapter creates a new NginxAdapter for the given LoadBalancerConfig.
func NewNginxAdapter(cfg config.NginxConfig) (*NginxAdapter, error) {
	if cfg.GetType() != "nginx" {
		return nil, fmt.Errorf("invalid load balancer type: %s, expected nginx", cfg.GetType())
	}

	clients, err := NewNginxClients(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create clients for nginx addresses %s: %w", cfg.Name, err)
	}

	// Initialize semaphore with RequestPoolSize
	sem := semaphore.NewWeighted(int64(cfg.GetRequestPoolSize()))

	return &NginxAdapter{
		clients:   clients,
		config:    cfg,
		semaphore: sem,
	}, nil
}

// ListBackends retrieves the current backends for a given upstream from all addresses.
func (a *NginxAdapter) ListBackends(ctx context.Context, upstreamName string) (map[string][]model.Backend, error) {
	clientBackends := make(map[string][]model.Backend)
	var errors *multierror.Error

	for _, client := range a.clients {
		path := fmt.Sprintf("dynamic?upstream=%s&verbose=", url.QueryEscape(upstreamName))
		body, err := client.doRequestWithRetry(ctx, "GET", path)
		if err != nil {
			errors = multierror.Append(errors, fmt.Errorf("client %s: failed to list backends for upstream %s: %w", client.baseURL, upstreamName, err))
			continue
		}

		// Parse NGINX response
		backends, err := parseNginxBackends(string(body))
		if err != nil {
			errors = multierror.Append(errors, fmt.Errorf("client %s: failed to parse backends for upstream %s: %w", client.baseURL, upstreamName, err))
			continue
		}

		// Add unique backends
		clientBackends[client.baseURL] = backends
	}

	if errors != nil {
		return nil, errors.ErrorOrNil()
	}
	return clientBackends, nil
}

// AddBackend adds a new backend to the specified upstream for all addresses or updates its weight.
func (a *NginxAdapter) AddBackend(ctx context.Context, upstreamName string, backend model.Backend) error {
	// Add upstream or Send weight update request to all addresses
	var errors *multierror.Error

	for _, client := range a.clients {
		path := fmt.Sprintf("dynamic?upstream=%s&verbose=", url.QueryEscape(upstreamName))
		body, err := client.doRequestWithRetry(ctx, "GET", path)
		if err != nil {
			errors = multierror.Append(errors, fmt.Errorf("client %s: failed to parse backends for upstream %s: %w", client.baseURL, upstreamName, err))
			continue
		}
		currentBackends, err := parseNginxBackends(string(body))
		if err != nil {
			errors = multierror.Append(errors, fmt.Errorf("client %s: failed to parse backends for upstream %s: %w", client.baseURL, upstreamName, err))
			continue
		}

		weight := -1
		for _, b := range currentBackends {
			if b.IP == backend.IP && b.Port == backend.Port {
				weight = b.Weight + 1
				break
			}
		}

		if weight == -1 {
			path = fmt.Sprintf("dynamic?upstream=%s&add=&server=%s:%d&weight=1", url.QueryEscape(upstreamName), url.QueryEscape(backend.IP), backend.Port)
		} else {
			path = fmt.Sprintf("dynamic?upstream=%s&server=%s:%d&weight=%d", url.QueryEscape(upstreamName), url.QueryEscape(backend.IP), backend.Port, weight)
		}
		_, err = client.doRequestWithRetry(ctx, "GET", path)
		if err != nil {
			errors = multierror.Append(errors, fmt.Errorf("client %s: failed to add/update backend %s:%d (weight=%d) to upstream %s: %w", client.baseURL, backend.IP, backend.Port, weight, upstreamName, err))
		}
	}
	return errors.ErrorOrNil()
}

// RemoveBackend removes a backend from the specified upstream for all addresses or decreases its weight.
func (a *NginxAdapter) RemoveBackend(ctx context.Context, upstreamName string, backend model.Backend) error {

	var errors *multierror.Error
	for _, client := range a.clients {
		path := fmt.Sprintf("dynamic?upstream=%s&verbose=", url.QueryEscape(upstreamName))
		body, err := client.doRequestWithRetry(ctx, "GET", path)
		if err != nil {
			errors = multierror.Append(errors, fmt.Errorf("client %s: failed to parse backends for upstream %s: %w", client.baseURL, upstreamName, err))
			continue
		}
		currentBackends, err := parseNginxBackends(string(body))
		if err != nil {
			errors = multierror.Append(errors, fmt.Errorf("client %s: failed to parse backends for upstream %s: %w", client.baseURL, upstreamName, err))
			continue
		}

		weight := -1
		for _, b := range currentBackends {
			if b.IP == backend.IP && b.Port == backend.Port {
				weight = b.Weight
				break
			}
		}

		if weight == -1 {
			errors = multierror.Append(errors, fmt.Errorf("backend %s:%d not found in upstream %s", backend.IP, backend.Port, upstreamName))
			continue
		}

		if weight < 2 {
			// Remove backend completely
			path = fmt.Sprintf("dynamic?upstream=%s&remove=&server=%s:%d", url.QueryEscape(upstreamName), url.QueryEscape(backend.IP), backend.Port)
		} else {
			// Decrease weight by 1
			weight = weight - 1
			path = fmt.Sprintf("dynamic?upstream=%s&server=%s:%d&weight=%d", url.QueryEscape(upstreamName), url.QueryEscape(backend.IP), backend.Port, weight)
		}
		_, err = client.doRequestWithRetry(ctx, "GET", path)
		if err != nil {
			errors = multierror.Append(errors, fmt.Errorf("client %s: failed to remove/update backend %s:%d (weight=%d) from upstream %s: %w", client.baseURL, backend.IP, backend.Port, weight, upstreamName, err))
		}

	}
	return errors.ErrorOrNil()
}

// SyncUpstream ensures the upstream matches the desired state for all addresses (idempotent).
func (a *NginxAdapter) SyncUpstream(ctx context.Context, upstream model.Upstream) error {
	// Get current backends
	desiredBackend := make(map[string]model.Backend)
	for _, db := range upstream.Backends {
		if tmpdb, exists := desiredBackend[fmt.Sprintf("%s:%d", db.IP, db.Port)]; exists {
			tmpdb.Weight = tmpdb.Weight + 1
			desiredBackend[fmt.Sprintf("%s:%d", db.IP, db.Port)] = tmpdb
		} else {
			db.Weight = 1
			desiredBackend[fmt.Sprintf("%s:%d", db.IP, db.Port)] = db
		}
	}

	var errors *multierror.Error
	for _, client := range a.clients {
		path := fmt.Sprintf("dynamic?upstream=%s&verbose=", url.QueryEscape(upstream.Name))
		body, err := client.doRequestWithRetry(ctx, "GET", path)
		if err != nil {
			errors = multierror.Append(errors, fmt.Errorf("client %s: failed to parse backends for upstream %s: %w", client.baseURL, upstream.Name, err))
			continue
		}
		currentBackends, err := parseNginxBackends(string(body))
		if err != nil {
			errors = multierror.Append(errors, fmt.Errorf("client %s: failed to parse backends for upstream %s: %w", client.baseURL, upstream.Name, err))
			continue
		}

		updateBackends := make(map[string]model.Backend)
		removeBackends := make(map[string]model.Backend)

		for key, db := range desiredBackend {
			for _, cb := range currentBackends {
				if db.IP == cb.IP && db.Port == cb.Port {
					if cb.Weight == db.Weight {
						db.Weight = -1
						desiredBackend[key] = db
					} else {
						updateBackends[fmt.Sprintf("%s:%d", db.IP, db.Port)] = db
						db.Weight = -1
						desiredBackend[key] = db
					}
				}
			}
		}

		for _, cb := range currentBackends {
			if _, addexists := desiredBackend[fmt.Sprintf("%s:%d", cb.IP, cb.Port)]; !addexists {
				removeBackends[fmt.Sprintf("%s:%d", cb.IP, cb.Port)] = cb
			}
		}

		for _, backend := range desiredBackend {
			if backend.Weight != -1 {
				path = fmt.Sprintf("dynamic?upstream=%s&add=&server=%s:%d&weight=%d", url.QueryEscape(upstream.Name), url.QueryEscape(backend.IP), backend.Port, backend.Weight)
				_, err = client.doRequestWithRetry(ctx, "GET", path)
				if err != nil {
					errors = multierror.Append(errors, fmt.Errorf("client %s: failed to add backend %s:%d (weight=%d) from upstream %s: %w", client.baseURL, backend.IP, backend.Port, backend.Weight, upstream.Name, err))
				}
			}
		}
		for _, backend := range updateBackends {
			path = fmt.Sprintf("dynamic?upstream=%s&server=%s:%d&weight=%d", url.QueryEscape(upstream.Name), url.QueryEscape(backend.IP), backend.Port, backend.Weight)
			_, err = client.doRequestWithRetry(ctx, "GET", path)
			if err != nil {
				errors = multierror.Append(errors, fmt.Errorf("client %s: failed to update backend %s:%d (weight=%d) from upstream %s: %w", client.baseURL, backend.IP, backend.Port, backend.Weight, upstream.Name, err))
			}
		}
		for _, backend := range removeBackends {
			path = fmt.Sprintf("dynamic?upstream=%s&remove=&server=%s:%d&weight=%d", url.QueryEscape(upstream.Name), url.QueryEscape(backend.IP), backend.Port, backend.Weight)
			_, err = client.doRequestWithRetry(ctx, "GET", path)
			if err != nil {
				errors = multierror.Append(errors, fmt.Errorf("client %s: failed to remove backend %s:%d from upstream %s: %w", client.baseURL, backend.IP, backend.Port, upstream.Name, err))
			}
		}

	}

	return errors.ErrorOrNil()
}

// parseNginxBackends parses NGINX response text into a list of Backends.
func parseNginxBackends(response string) ([]model.Backend, error) {
	var backends []model.Backend
	// Updated regex to handle optional 's' in fail_timeout and flexible order of parameters
	re := regexp.MustCompile(`server\s+([\d.]+):(\d+)\s+weight=(\d+)(?:\s+max_fails=\d+)?(?:\s+fail_timeout=\d+(?:s)?)?(?:\s+(?:down|backup))*\s*;`)

	lines := strings.Split(response, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		matches := re.FindStringSubmatch(line)
		if len(matches) < 4 {
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
