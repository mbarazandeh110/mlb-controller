// Package nginx provides an adapter for interacting with NGINX load balancers.
package nginx

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"sync"

	"mlb-controller/internal/domain/config"
	"mlb-controller/internal/domain/model"

	"github.com/hashicorp/go-multierror"
	"golang.org/x/sync/semaphore"
)

// NginxAdapter implements the LoadBalancerAdapter interface for NGINX.
// It manages multiple NGINX clients and uses a semaphore for concurrency control.
type NginxAdapter struct {
	clients   []*NginxClient            // List of NGINX clients for each address
	config    config.LoadBalancerConfig // Load balancer configuration
	semaphore *semaphore.Weighted       // Semaphore to limit concurrent requests
}

// NewNginxAdapter creates a new NginxAdapter for the given LoadBalancerConfig.
// It validates the configuration type, initializes NGINX clients, and sets up the semaphore.
func NewNginxAdapter(cfg config.NginxConfig) (*NginxAdapter, error) {
	// Validate that the configuration type is "nginx"
	if cfg.GetType() != "nginx" {
		return nil, fmt.Errorf("invalid load balancer type: %s, expected nginx", cfg.GetType())
	}

	// Initialize NGINX clients based on the provided addresses
	clients, err := NewNginxClientSet(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create clients for nginx addresses %s: %w", cfg.Name, err)
	}

	// Initialize semaphore with the configured request pool size
	sem := semaphore.NewWeighted(int64(cfg.GetRequestPoolSize()))

	// Return a new NginxAdapter instance
	return &NginxAdapter{
		clients:   clients,
		config:    cfg,
		semaphore: sem,
	}, nil
}

// ListBackends retrieves the current backends for a given upstream from all NGINX addresses.
// It uses goroutines for concurrent requests and a semaphore to limit concurrency.
// Returns a map of backends keyed by client baseURL, or an error if any request fails.
func (a *NginxAdapter) ListBackends(ctx context.Context, upstreamName string) (map[string][]model.Backend, error) {
	// Initialize shared data structures
	clientBackends := make(map[string][]model.Backend)
	var mu sync.Mutex            // Mutex to protect shared map and errors
	var errors *multierror.Error // Accumulate errors from all goroutines
	var wg sync.WaitGroup        // WaitGroup to synchronize goroutines

	// Launch a goroutine for each client
	for _, client := range a.clients {
		wg.Add(1)
		go func(client *NginxClient) {
			defer wg.Done()

			// Acquire a semaphore slot to limit concurrent requests
			if err := a.semaphore.Acquire(ctx, 1); err != nil {
				// Lock mutex to safely append error
				mu.Lock()
				errors = multierror.Append(errors, fmt.Errorf("client %s: failed to acquire semaphore: %w", client.baseURL, err))
				mu.Unlock()
				return
			}
			defer a.semaphore.Release(1) // Release semaphore slot after completion

			// Fetch current backends for the upstream
			currentBackends, err := a.getCurrentBackends(ctx, client, upstreamName)
			if err != nil {
				// Lock mutex to safely append error
				mu.Lock()
				errors = multierror.Append(errors, fmt.Errorf("client %s: failed to parse backends for upstream %s: %w", client.baseURL, upstreamName, err))
				mu.Unlock()
				return
			}

			// Lock mutex to safely update shared map
			mu.Lock()
			clientBackends[client.baseURL] = currentBackends
			mu.Unlock()
		}(client)
	}

	// Wait for all goroutines to complete
	wg.Wait()

	// Return accumulated errors and the collected backends
	return clientBackends, errors.ErrorOrNil()
}

// AddBackend adds a new backend to the specified upstream for all NGINX addresses or updates its weight.
// It uses goroutines for concurrent requests and a semaphore to limit concurrency.
// Returns an error if any request fails.
func (a *NginxAdapter) AddBackend(ctx context.Context, upstreamName string, backend model.Backend) error {
	var errors *multierror.Error // Accumulate errors from all goroutines
	var mu sync.Mutex            // Mutex to protect shared errors
	var wg sync.WaitGroup        // WaitGroup to synchronize goroutines

	// Launch a goroutine for each client
	for _, client := range a.clients {
		wg.Add(1)
		go func(client *NginxClient) {
			defer wg.Done()

			// Acquire a semaphore slot to limit concurrent requests
			if err := a.semaphore.Acquire(ctx, 1); err != nil {
				// Lock mutex to safely append error
				mu.Lock()
				errors = multierror.Append(errors, fmt.Errorf("client %s: failed to acquire semaphore: %w", client.baseURL, err))
				mu.Unlock()
				return
			}
			defer a.semaphore.Release(1) // Release semaphore slot after completion

			// Fetch current backends to determine if the backend exists
			currentBackends, err := a.getCurrentBackends(ctx, client, upstreamName)
			if err != nil {
				// Lock mutex to safely append error
				mu.Lock()
				errors = multierror.Append(errors, fmt.Errorf("client %s: failed to parse backends for upstream %s: %w", client.baseURL, upstreamName, err))
				mu.Unlock()
				return
			}

			// Check if backend exists and determine new weight
			weight := -1
			for _, b := range currentBackends {
				if b.IP == backend.IP && b.Port == backend.Port {
					weight = b.Weight + 1
					break
				}
			}

			// Construct request path based on whether backend exists
			path := ""
			if weight == -1 {
				// Add new backend with weight=1
				path = fmt.Sprintf("dynamic?upstream=%s&add=&server=%s:%d&weight=1", url.QueryEscape(upstreamName), url.QueryEscape(backend.IP), backend.Port)
			} else {
				// Update existing backend with incremented weight
				path = fmt.Sprintf("dynamic?upstream=%s&server=%s:%d&weight=%d", url.QueryEscape(upstreamName), url.QueryEscape(backend.IP), backend.Port, weight)
			}

			// Execute the request
			_, err = client.doRequest(ctx, "GET", path)
			if err != nil {
				// Lock mutex to safely append error
				mu.Lock()
				errors = multierror.Append(errors, fmt.Errorf("client %s: failed to add/update backend %s:%d (weight=%d) to upstream %s: %w", client.baseURL, backend.IP, backend.Port, weight, upstreamName, err))
				mu.Unlock()
			}
		}(client)
	}

	// Wait for all goroutines to complete
	wg.Wait()
	return errors.ErrorOrNil()
}

// RemoveBackend removes a backend from the specified upstream for all NGINX addresses or decreases its weight.
// It uses goroutines for concurrent requests and a semaphore to limit concurrency.
// Returns an error if any request fails.
func (a *NginxAdapter) RemoveBackend(ctx context.Context, upstreamName string, backend model.Backend) error {
	var errors *multierror.Error // Accumulate errors from all goroutines
	var mu sync.Mutex            // Mutex to protect shared errors
	var wg sync.WaitGroup        // WaitGroup to synchronize goroutines

	// Launch a goroutine for each client
	for _, client := range a.clients {
		wg.Add(1)
		go func(client *NginxClient) {
			defer wg.Done()

			// Acquire a semaphore slot to limit concurrent requests
			if err := a.semaphore.Acquire(ctx, 1); err != nil {
				// Lock mutex to safely append error
				mu.Lock()
				errors = multierror.Append(errors, fmt.Errorf("client %s: failed to acquire semaphore: %w", client.baseURL, err))
				mu.Unlock()
				return
			}
			defer a.semaphore.Release(1) // Release semaphore slot after completion

			// Fetch current backends to find the backend
			currentBackends, err := a.getCurrentBackends(ctx, client, upstreamName)
			if err != nil {
				// Lock mutex to safely append error
				mu.Lock()
				errors = multierror.Append(errors, fmt.Errorf("client %s: failed to parse backends for upstream %s: %w", client.baseURL, upstreamName, err))
				mu.Unlock()
				return
			}

			// Check if backend exists and get its weight
			weight := -1
			for _, b := range currentBackends {
				if b.IP == backend.IP && b.Port == backend.Port {
					weight = b.Weight
					break
				}
			}

			// If backend not found, append error
			if weight == -1 {
				mu.Lock()
				errors = multierror.Append(errors, fmt.Errorf("backend %s:%d not found in upstream %s", backend.IP, backend.Port, upstreamName))
				mu.Unlock()
				return
			}

			// Construct request path based on weight
			path := ""
			if weight < 2 {
				// Remove backend completely if weight is 1
				path = fmt.Sprintf("dynamic?upstream=%s&remove=&server=%s:%d", url.QueryEscape(upstreamName), url.QueryEscape(backend.IP), backend.Port)
			} else {
				// Decrease weight by 1
				weight = weight - 1
				path = fmt.Sprintf("dynamic?upstream=%s&server=%s:%d&weight=%d", url.QueryEscape(upstreamName), url.QueryEscape(backend.IP), backend.Port, weight)
			}

			// Execute the request
			_, err = client.doRequest(ctx, "GET", path)
			if err != nil {
				// Lock mutex to safely append error
				mu.Lock()
				errors = multierror.Append(errors, fmt.Errorf("client %s: failed to remove/update backend %s:%d (weight=%d) from upstream %s: %w", client.baseURL, backend.IP, backend.Port, weight, upstreamName, err))
				mu.Unlock()
			}
		}(client)
	}

	// Wait for all goroutines to complete
	wg.Wait()
	return errors.ErrorOrNil()
}

// SyncUpstream ensures the upstream matches the desired state for all NGINX addresses (idempotent).
// It adds, updates, or removes backends as needed using goroutines and a semaphore for concurrency control.
// Returns an error if any request fails.
func (a *NginxAdapter) SyncUpstream(ctx context.Context, upstream model.Upstream) error {
	// Create a map of desired backends with aggregated weights
	desiredBackend := make(map[string]model.Backend)
	for _, db := range upstream.Backends {
		key := fmt.Sprintf("%s:%d", db.IP, db.Port)
		if tmpdb, exists := desiredBackend[key]; exists {
			// Increment weight for duplicate backends
			tmpdb.Weight = tmpdb.Weight + 1
			desiredBackend[key] = tmpdb
		} else {
			// Initialize new backend with weight=1
			if db.Weight < 1 {
				db.Weight = 1
			}
			desiredBackend[key] = db
		}
	}

	var errors *multierror.Error // Accumulate errors from all goroutines
	var mu sync.Mutex            // Mutex to protect shared errors
	var wg sync.WaitGroup        // WaitGroup to synchronize goroutines

	// Launch a goroutine for each client
	for _, client := range a.clients {
		wg.Add(1)
		go func(client *NginxClient) {
			defer wg.Done()

			// Acquire a semaphore slot to limit concurrent requests
			if err := a.semaphore.Acquire(ctx, 1); err != nil {
				// Lock mutex to safely append error
				mu.Lock()
				errors = multierror.Append(errors, fmt.Errorf("client %s: failed to acquire semaphore: %w", client.baseURL, err))
				mu.Unlock()
				return
			}
			defer a.semaphore.Release(1) // Release semaphore slot after completion

			// Fetch current backends
			currentBackends, err := a.getCurrentBackends(ctx, client, upstream.Name)
			if err != nil {
				// Lock mutex to safely append error
				mu.Lock()
				errors = multierror.Append(errors, fmt.Errorf("client %s: failed to parse backends for upstream %s: %w", client.baseURL, upstream.Name, err))
				mu.Unlock()
				return
			}

			// Determine which backends to add, update, or remove
			updateBackends := make(map[string]model.Backend)
			removeBackends := make(map[string]model.Backend)

			// Compare desired and current backends
			for key, db := range desiredBackend {
				for _, cb := range currentBackends {
					if db.IP == cb.IP && db.Port == cb.Port {
						if cb.Weight == db.Weight {
							// No change needed, mark as processed
							db.Weight = -1
							desiredBackend[key] = db
						} else {
							// Weight differs, mark for update
							updateBackends[fmt.Sprintf("%s:%d", db.IP, db.Port)] = db
							db.Weight = -1
							desiredBackend[key] = db
						}
					}
				}
			}

			// Identify backends to remove (present in current but not in desired)
			for _, cb := range currentBackends {
				if _, addexists := desiredBackend[fmt.Sprintf("%s:%d", cb.IP, cb.Port)]; !addexists {
					removeBackends[fmt.Sprintf("%s:%d", cb.IP, cb.Port)] = cb
				}
			}

			// Add new backends
			for _, backend := range desiredBackend {
				if backend.Weight != -1 {
					path := fmt.Sprintf("dynamic?upstream=%s&add=&server=%s:%d&weight=%d", url.QueryEscape(upstream.Name), url.QueryEscape(backend.IP), backend.Port, backend.Weight)
					_, err = client.doRequest(ctx, "GET", path)
					if err != nil {
						// Lock mutex to safely append error
						mu.Lock()
						errors = multierror.Append(errors, fmt.Errorf("client %s: failed to add backend %s:%d (weight=%d) from upstream %s: %w", client.baseURL, backend.IP, backend.Port, backend.Weight, upstream.Name, err))
						mu.Unlock()
					}
				}
			}

			// Update existing backends
			for _, backend := range updateBackends {
				path := fmt.Sprintf("dynamic?upstream=%s&server=%s:%d&weight=%d", url.QueryEscape(upstream.Name), url.QueryEscape(backend.IP), backend.Port, backend.Weight)
				_, err = client.doRequest(ctx, "GET", path)
				if err != nil {
					// Lock mutex to safely append error
					mu.Lock()
					errors = multierror.Append(errors, fmt.Errorf("client %s: failed to update backend %s:%d (weight=%d) from upstream %s: %w", client.baseURL, backend.IP, backend.Port, backend.Weight, upstream.Name, err))
					mu.Unlock()
				}
			}

			// Remove backends
			for _, backend := range removeBackends {
				path := fmt.Sprintf("dynamic?upstream=%s&remove=&server=%s:%d&weight=%d", url.QueryEscape(upstream.Name), url.QueryEscape(backend.IP), backend.Port, backend.Weight)
				_, err = client.doRequest(ctx, "GET", path)
				if err != nil {
					// Lock mutex to safely append error
					mu.Lock()
					errors = multierror.Append(errors, fmt.Errorf("client %s: failed to remove backend %s:%d from upstream %s: %w", client.baseURL, backend.IP, backend.Port, upstream.Name, err))
					mu.Unlock()
				}
			}
		}(client)
	}

	// Wait for all goroutines to complete
	wg.Wait()
	return errors.ErrorOrNil()
}

// parseNginxBackends parses NGINX response text into a list of Backends.
// It uses a regex to extract backend details (IP, port, weight) from the response.
func parseNginxBackends(response string) ([]model.Backend, error) {
	var backends []model.Backend
	// Regex to match server lines with IP, port, weight, and optional parameters
	// re := regexp.MustCompile(`server\s+([\d.]+):(\d+)(?:\s+weight=(\d+))?.*?;`) // ipv4
	re := regexp.MustCompile(`server\s+([0-9a-fA-F.:\[\]]+):(\d+)(?:\s+weight=(\d+))?.*?;`) // ipv4,ipv6

	// Split response into lines
	lines := strings.Split(response, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue // Skip empty lines
		}
		matches := re.FindStringSubmatch(line)
		if len(matches) < 4 {
			return nil, fmt.Errorf("invalid backend format: %s", line)
		}

		// Extract IP, port, and weight
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

		// Append parsed backend to the list
		backends = append(backends, model.Backend{
			IP:     ip,
			Port:   int32(port),
			Weight: weight,
		})
	}

	return backends, nil
}

// getCurrentBackends retrieves the current backends for a given upstream from a single NGINX client.
// It sends a request to the NGINX API and parses the response using parseNginxBackends.
func (a *NginxAdapter) getCurrentBackends(ctx context.Context, client *NginxClient, upstreamName string) ([]model.Backend, error) {
	// Construct the request path for listing backends
	path := fmt.Sprintf("dynamic?upstream=%s&verbose=", url.QueryEscape(upstreamName))
	body, err := client.doRequest(ctx, "GET", path)
	if err != nil {
		return nil, fmt.Errorf("client %s: failed to parse backends for upstream %s: %w", client.baseURL, upstreamName, err)
	}
	return parseNginxBackends(string(body))
}
