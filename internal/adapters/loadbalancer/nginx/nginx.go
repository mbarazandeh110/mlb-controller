// Package nginx provides an adapter for interacting with NGINX load balancers.
package nginx

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"sort"
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
		return nil, fmt.Errorf("failed to bulk fetch upstreams from %s: %w", client.baseURL, err)
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
	// Aggregate duplicate backends (e.g. multiple Pods on the same Node) by incrementing weight
	desiredBackend := make(map[string]model.Backend)
	for _, db := range upstream.Backends {
		key := backendKey(db)
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

			// Get current backends for this upstream (support both named and single flat format)
			currentBackends := allBackends[upstream.Name]
			if len(currentBackends) == 0 && len(allBackends[""]) > 0 {
				currentBackends = allBackends[""]
			}

			// Safety Net check: If desired has 0 backends and current has > 0 backends,
			// and AllowEmptyUpstream is false, skip draining to prevent outage!
			if len(upstream.Backends) == 0 && len(currentBackends) > 0 && !a.config.AllowEmptyUpstream {
				a.logger.Warn("Safety Net triggered: skipping backend draining because allow_empty_upstream is false",
					logging.Field{Key: "client", Value: c.baseURL},
					logging.Field{Key: "upstream", Value: upstream.Name},
					logging.Field{Key: "current_backends", Value: len(currentBackends)})
				return
			}

			updateBackends := make(map[string]model.Backend)
			removeBackends := make(map[string]model.Backend)
			changed := false

			for key, db := range desiredBackend {
				matched := false
				for _, cb := range currentBackends {
					if sameBackend(db, cb) {
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
				key := backendKey(cb)
				if _, exists := desiredBackend[key]; !exists {
					removeBackends[key] = cb
					changed = true
				}
			}

			// Deterministic ordering for add/update requests
			updateKeys := make([]string, 0, len(updateBackends))
			for k := range updateBackends {
				updateKeys = append(updateKeys, k)
			}
			sort.Strings(updateKeys)

			for _, k := range updateKeys {
				backend := updateBackends[k]
				serverAddr := formatServerAddr(backend.IP, backend.Port)
				path := a.buildAddPath(upstream.Name, serverAddr, backend.Weight)

				a.logger.Debug("Syncing backend to NGINX",
					logging.Field{Key: "client", Value: c.baseURL},
					logging.Field{Key: "upstream", Value: upstream.Name},
					logging.Field{Key: "backend", Value: serverAddr},
					logging.Field{Key: "weight", Value: backend.Weight})

				if _, err := c.doRequest(ctx, "GET", path); err != nil {
					a.logger.Error("Failed to add/update backend on NGINX",
						logging.Field{Key: "client", Value: c.baseURL},
						logging.Field{Key: "upstream", Value: upstream.Name},
						logging.Field{Key: "backend", Value: serverAddr},
						logging.Field{Key: "error", Value: err})
					mu.Lock()
					errors = multierror.Append(errors, fmt.Errorf("client %s add failed: %w", c.baseURL, err))
					mu.Unlock()
				} else {
					a.logger.Info("Successfully synced backend to NGINX",
						logging.Field{Key: "client", Value: c.baseURL},
						logging.Field{Key: "upstream", Value: upstream.Name},
						logging.Field{Key: "backend", Value: serverAddr},
						logging.Field{Key: "weight", Value: backend.Weight})
				}
			}

			// Deterministic ordering for remove requests
			removeKeys := make([]string, 0, len(removeBackends))
			for k := range removeBackends {
				removeKeys = append(removeKeys, k)
			}
			sort.Strings(removeKeys)

			for _, k := range removeKeys {
				backend := removeBackends[k]
				serverAddr := formatServerAddr(backend.IP, backend.Port)
				path := a.buildRemovePath(upstream.Name, serverAddr)

				a.logger.Debug("Removing backend from NGINX",
					logging.Field{Key: "client", Value: c.baseURL},
					logging.Field{Key: "upstream", Value: upstream.Name},
					logging.Field{Key: "backend", Value: serverAddr})

				if _, err := c.doRequest(ctx, "GET", path); err != nil {
					a.logger.Error("Failed to remove backend from NGINX",
						logging.Field{Key: "client", Value: c.baseURL},
						logging.Field{Key: "upstream", Value: upstream.Name},
						logging.Field{Key: "backend", Value: serverAddr},
						logging.Field{Key: "error", Value: err})
					mu.Lock()
					errors = multierror.Append(errors, fmt.Errorf("client %s remove failed: %w", c.baseURL, err))
					mu.Unlock()
				} else {
					a.logger.Info("Successfully removed backend from NGINX",
						logging.Field{Key: "client", Value: c.baseURL},
						logging.Field{Key: "upstream", Value: upstream.Name},
						logging.Field{Key: "backend", Value: serverAddr})
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
// این متد توسط کنترلر در بازه‌های زمانی مشخص فراخوانی می‌شود تا آپ‌استریم‌هایی که از K8s پاک شده‌اند، از NGINX نیز حذف شوند.
func (a *NginxAdapter) SyncAllUpstreams(ctx context.Context, desiredUpstreams map[string]model.Upstream) error {
	var errors *multierror.Error

	// 1. Reconcile all desired upstreams to actively fix any state drift
	for _, desiredUp := range desiredUpstreams {
		if err := a.SyncUpstream(ctx, desiredUp); err != nil {
			errors = multierror.Append(errors, fmt.Errorf("sync upstream %s failed: %w", desiredUp.Name, err))
		}
	}

	// 2. If PruneOrphans is enabled, find and remove upstreams present on NGINX that are not in desiredUpstreams
	if !a.config.PruneOrphans {
		return errors.ErrorOrNil()
	}

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

			allNginxUpstreams, err := a.refreshCache(ctx, c)
			if err != nil {
				mu.Lock()
				errors = multierror.Append(errors, err)
				mu.Unlock()
				return
			}

			// Sort upstream names for deterministic processing
			upstreamNames := make([]string, 0, len(allNginxUpstreams))
			for name := range allNginxUpstreams {
				upstreamNames = append(upstreamNames, name)
			}
			sort.Strings(upstreamNames)

			// یافتن آپ‌استریم‌های یتیم (Orphaned) که در NGINX هستند اما در K8s وجود ندارند
			changed := false
			for _, upstreamName := range upstreamNames {
				if upstreamName == "" {
					continue
				}
				backends := allNginxUpstreams[upstreamName]
				_, exists := desiredUpstreams[upstreamName]
				if !exists {
					for _, backend := range backends {
						serverAddr := formatServerAddr(backend.IP, backend.Port)
						path := a.buildRemovePath(upstreamName, serverAddr)

						a.logger.Debug("Removing orphaned backend from NGINX",
							logging.Field{Key: "client", Value: c.baseURL},
							logging.Field{Key: "upstream", Value: upstreamName},
							logging.Field{Key: "backend", Value: serverAddr})

						if _, err := c.doRequest(ctx, "GET", path); err != nil {
							a.logger.Error("Failed to remove orphaned backend from NGINX",
								logging.Field{Key: "client", Value: c.baseURL},
								logging.Field{Key: "upstream", Value: upstreamName},
								logging.Field{Key: "backend", Value: serverAddr},
								logging.Field{Key: "error", Value: err})
							mu.Lock()
							errors = multierror.Append(errors, fmt.Errorf("client %s remove orphaned backend %s from %s failed: %w", c.baseURL, serverAddr, upstreamName, err))
							mu.Unlock()
						} else {
							changed = true
							a.logger.Info("Successfully removed orphaned backend from NGINX",
								logging.Field{Key: "client", Value: c.baseURL},
								logging.Field{Key: "upstream", Value: upstreamName},
								logging.Field{Key: "backend", Value: serverAddr})
						}
					}
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

func (a *NginxAdapter) buildAddPath(upstreamName, serverAddr string, weight int) string {
	api := strings.TrimPrefix(a.config.AddAPI, "/")
	delim := "?"
	if strings.Contains(api, "?") {
		delim = "&"
	}
	path := fmt.Sprintf("%s%supstream=%s&server=%s&weight=%d",
		api,
		delim,
		url.QueryEscape(upstreamName),
		url.QueryEscape(serverAddr),
		weight)
	if !strings.Contains(api, "add") {
		path += "&add="
	}
	return path
}

func (a *NginxAdapter) buildRemovePath(upstreamName, serverAddr string) string {
	api := strings.TrimPrefix(a.config.RemoveAPI, "/")
	delim := "?"
	if strings.Contains(api, "?") {
		delim = "&"
	}
	path := fmt.Sprintf("%s%supstream=%s&server=%s",
		api,
		delim,
		url.QueryEscape(upstreamName),
		url.QueryEscape(serverAddr))
	if !strings.Contains(api, "remove") {
		path += "&remove="
	}
	return path
}

// پارس کردن خروجی متنی Bulk NGINX و نگاشت آن به ساختار Map
// پشتیبانی از فرمت Multi-Upstream و Single-Upstream Flat، آدرس‌های IPv4 و IPv6، و پارامترهای اختیاری وزن
func parseBulkNginxBackends(response string) map[string][]model.Backend {
	result := make(map[string][]model.Backend)
	lines := strings.Split(response, "\n")
	var currentUpstream string

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") {
			continue
		}

		if strings.HasPrefix(line, "server ") {
			line = strings.TrimPrefix(line, "server ")
			line = strings.TrimSpace(line)
			line = strings.TrimSuffix(line, ";")

			fields := strings.Fields(line)
			if len(fields) == 0 {
				continue
			}

			hostPort := fields[0]
			host, portStr, err := net.SplitHostPort(hostPort)
			if err != nil {
				continue
			}
			port, err := strconv.Atoi(portStr)
			if err != nil {
				continue
			}

			weight := 1
			for _, f := range fields[1:] {
				if strings.HasPrefix(f, "weight=") {
					if w, err := strconv.Atoi(strings.TrimPrefix(f, "weight=")); err == nil && w > 0 {
						weight = w
					}
					break
				}
			}

			// Preserve bracket format for IPv6 if originally bracketed
			ip := host
			if strings.Contains(ip, ":") && strings.HasPrefix(hostPort, "[") {
				ip = fmt.Sprintf("[%s]", host)
			}

			backend := model.Backend{
				IP:     ip,
				Port:   int32(port),
				Weight: weight,
			}

			result[currentUpstream] = append(result[currentUpstream], backend)
		} else if strings.HasSuffix(line, ":") || strings.HasPrefix(line, "upstream ") || strings.HasSuffix(line, "{") {
			// Detection of new upstream name header, e.g. "upstream_name:" or "upstream upstream_name {"
			trimmed := strings.TrimSuffix(line, ":")
			trimmed = strings.TrimPrefix(trimmed, "upstream ")
			trimmed = strings.TrimSuffix(trimmed, "{")
			currentUpstream = strings.TrimSpace(trimmed)
		}
	}
	return result
}

// Helper functions for address formatting and comparison

func cleanIP(ip string) string {
	return strings.Trim(strings.TrimSpace(ip), "[]")
}

func sameBackend(b1, b2 model.Backend) bool {
	return cleanIP(b1.IP) == cleanIP(b2.IP) && b1.Port == b2.Port
}

func backendKey(b model.Backend) string {
	return fmt.Sprintf("%s:%d", cleanIP(b.IP), b.Port)
}

func formatServerAddr(ip string, port int32) string {
	cIP := cleanIP(ip)
	if strings.Contains(cIP, ":") {
		return fmt.Sprintf("[%s]:%d", cIP, port)
	}
	return fmt.Sprintf("%s:%d", cIP, port)
}
