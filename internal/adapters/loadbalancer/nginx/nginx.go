// Package nginx provides an adapter for interacting with NGINX load balancers.
package nginx

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"mlb-controller/internal/domain/config"
	"mlb-controller/internal/domain/model"
	"mlb-controller/internal/ports/logging"

	"github.com/hashicorp/go-multierror"
	"golang.org/x/sync/semaphore"
)

type NginxAdapter struct {
	clients   []*NginxClient
	config    config.NginxConfig
	logger    logging.Logger
	semaphore *semaphore.Weighted

	// مکانیزم In-Memory Cache برای جلوگیری از بمباران API (رفع خطای O(N))
	cache      map[string]map[string][]model.Backend // clientBaseURL -> upstreamName -> backends
	cacheExp   time.Time
	cacheMutex sync.RWMutex
}

func NewNginxAdapter(cfg config.NginxConfig, logger logging.Logger) (*NginxAdapter, error) {
	if cfg.GetType() != "nginx" {
		return nil, fmt.Errorf("invalid load balancer type: %s, expected nginx", cfg.GetType())
	}

	clients, err := NewNginxClientSet(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create clients for nginx addresses %s: %w", cfg.Name, err)
	}

	sem := semaphore.NewWeighted(int64(cfg.GetRequestPoolSize()))

	return &NginxAdapter{
		clients:   clients,
		config:    cfg,
		logger:    logger,
		semaphore: sem,
		cache:     make(map[string]map[string][]model.Backend),
	}, nil
}

// واکشی Bulk کل آپ‌استریم‌ها و ذخیره در کش با TTL دو ثانیه‌ای
func (a *NginxAdapter) refreshCache(ctx context.Context, client *NginxClient) (map[string][]model.Backend, error) {
	a.cacheMutex.RLock()
	if time.Now().Before(a.cacheExp) && a.cache[client.baseURL] != nil {
		defer a.cacheMutex.RUnlock()
		return a.cache[client.baseURL], nil
	}
	a.cacheMutex.RUnlock()

	// استفاده از ListAPI تعریف شده در کانفیگ به جای Hardcode
	path := strings.TrimPrefix(a.config.ListAPI, "/")
	body, err := client.doRequest(ctx, "GET", path)
	if err != nil {
		return nil, fmt.Errorf("failed to bulk fetch upstreams: %w", err)
	}

	parsedState := parseBulkNginxBackends(string(body))

	a.cacheMutex.Lock()
	a.cache[client.baseURL] = parsedState
	a.cacheExp = time.Now().Add(2 * time.Second)
	a.cacheMutex.Unlock()

	return parsedState, nil
}

func (a *NginxAdapter) invalidateCache(clientBaseURL string) {
	a.cacheMutex.Lock()
	delete(a.cache, clientBaseURL)
	a.cacheMutex.Unlock()
}

func (a *NginxAdapter) SyncUpstream(ctx context.Context, upstream model.Upstream) error {
	desiredBackend := make(map[string]model.Backend)
	for _, db := range upstream.Backends {
		key := fmt.Sprintf("%s:%d", db.IP, db.Port)
		if tmpdb, exists := desiredBackend[key]; exists {
			tmpdb.Weight += 1
			desiredBackend[key] = tmpdb
		} else {
			if db.Weight < 1 {
				db.Weight = 1
			}
			desiredBackend[key] = db
		}
	}

	var errors *multierror.Error
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, client := range a.clients {
		wg.Add(1)
		go func(c *NginxClient) {
			defer wg.Done()

			if err := a.semaphore.Acquire(ctx, 1); err != nil {
				mu.Lock()
				errors = multierror.Append(errors, fmt.Errorf("client %s semaphore error: %w", c.baseURL, err))
				mu.Unlock()
				return
			}
			defer a.semaphore.Release(1)

			allBackends, err := a.refreshCache(ctx, c)
			if err != nil {
				mu.Lock()
				errors = multierror.Append(errors, err)
				mu.Unlock()
				return
			}
			currentBackends := allBackends[upstream.Name]

			updateBackends := make(map[string]model.Backend)
			removeBackends := make(map[string]model.Backend)
			changed := false

			for key, db := range desiredBackend {
				matched := false
				for _, cb := range currentBackends {
					if db.IP == cb.IP && db.Port == cb.Port {
						matched = true
						if cb.Weight != db.Weight {
							updateBackends[key] = db
							changed = true
						}
						break
					}
				}
				if !matched {
					updateBackends[key] = db
					changed = true
				}
			}

			for _, cb := range currentBackends {
				if _, exists := desiredBackend[fmt.Sprintf("%s:%d", cb.IP, cb.Port)]; !exists {
					removeBackends[fmt.Sprintf("%s:%d", cb.IP, cb.Port)] = cb
					changed = true
				}
			}

			// Add/Update
			for _, backend := range updateBackends {
				path := fmt.Sprintf("%s?upstream=%s&server=%s:%d&weight=%d",
					strings.TrimPrefix(a.config.AddAPI, "/"),
					url.QueryEscape(upstream.Name),
					url.QueryEscape(backend.IP),
					backend.Port,
					backend.Weight)
				
				if _, err := c.doRequest(ctx, "GET", path); err != nil {
					mu.Lock()
					errors = multierror.Append(errors, fmt.Errorf("client %s add failed: %w", c.baseURL, err))
					mu.Unlock()
				}
			}

			// Remove
			for _, backend := range removeBackends {
				path := fmt.Sprintf("%s?upstream=%s&server=%s:%d",
					strings.TrimPrefix(a.config.RemoveAPI, "/"),
					url.QueryEscape(upstream.Name),
					url.QueryEscape(backend.IP),
					backend.Port)
				
				if _, err := c.doRequest(ctx, "GET", path); err != nil {
					mu.Lock()
					errors = multierror.Append(errors, fmt.Errorf("client %s remove failed: %w", c.baseURL, err))
					mu.Unlock()
				}
			}

			if changed {
				a.invalidateCache(c.baseURL)
			}
		}(client)
	}

	wg.Wait()
	return errors.ErrorOrNil()
}

// پیاده‌سازی اصلی Two-Way Reconciliation برای مقابله با State Drift
// این متد باید توسط کنترلر در بازه‌های زمانی مشخص فراخوانی شود تا آپ‌استریم‌هایی که از K8s پاک شده‌اند، از NGINX نیز حذف شوند.
func (a *NginxAdapter) SyncAllUpstreams(ctx context.Context, desiredUpstreams map[string]model.Upstream) error {
	var errors *multierror.Error
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, client := range a.clients {
		wg.Add(1)
		go func(c *NginxClient) {
			defer wg.Done()
			allNginxUpstreams, err := a.refreshCache(ctx, c)
			if err != nil {
				mu.Lock()
				errors = multierror.Append(errors, err)
				mu.Unlock()
				return
			}

			// یافتن آپ‌استریم‌های یتیم (Orphaned) که در NGINX هستند اما در K8s وجود ندارند
			for upstreamName, backends := range allNginxUpstreams {
				if _, exists := desiredUpstreams[upstreamName]; !exists {
					for _, backend := range backends {
						path := fmt.Sprintf("%s?upstream=%s&server=%s:%d",
							strings.TrimPrefix(a.config.RemoveAPI, "/"),
							url.QueryEscape(upstreamName),
							url.QueryEscape(backend.IP),
							backend.Port)
						
						if _, err := c.doRequest(ctx, "GET", path); err != nil {
							a.logger.Error("Failed to remove orphaned backend", logging.Field{Key: "error", Value: err})
						}
					}
					a.invalidateCache(c.baseURL)
				}
			}
		}(client)
	}

	wg.Wait()
	return errors.ErrorOrNil()
}

// پارس کردن خروجی متنی Bulk NGINX و نگاشت آن به ساختار Map
func parseBulkNginxBackends(response string) map[string][]model.Backend {
	result := make(map[string][]model.Backend)
	lines := strings.Split(response, "\n")
	var currentUpstream string

	reServer := regexp.MustCompile(`server\s+([0-9a-fA-F.:\[\]]+):(\d+)(?:\s+weight=(\d+))?`)

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		if strings.HasPrefix(line, "server ") {
			if currentUpstream == "" {
				continue
			}
			matches := reServer.FindStringSubmatch(line)
			if len(matches) >= 3 {
				ip := matches[1]
				port, _ := strconv.Atoi(matches[2])
				weight := 1
				if len(matches) >= 4 && matches[3] != "" {
					weight, _ = strconv.Atoi(matches[3])
				}
				result[currentUpstream] = append(result[currentUpstream], model.Backend{
					IP:     ip,
					Port:   int32(port),
					Weight: weight,
				})
			}
		} else {
			// تشخیص نام آپ‌استریم جدید
			currentUpstream = strings.TrimSuffix(line, ":") 
		}
	}
	return result
}