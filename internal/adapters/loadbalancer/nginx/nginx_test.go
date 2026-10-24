package nginx

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"mlb-controller/internal/domain/config"
	"mlb-controller/internal/domain/model"
	"mlb-controller/internal/ports/logging"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeLogger struct {
	mu       sync.Mutex
	messages []string
}

func (f *fakeLogger) Debug(msg string, fields ...logging.Field) { f.log("DEBUG: " + msg) }
func (f *fakeLogger) Info(msg string, fields ...logging.Field)  { f.log("INFO: " + msg) }
func (f *fakeLogger) Warn(msg string, fields ...logging.Field)  { f.log("WARN: " + msg) }
func (f *fakeLogger) Error(msg string, fields ...logging.Field) { f.log("ERROR: " + msg) }
func (f *fakeLogger) Fatal(msg string, fields ...logging.Field) { f.log("FATAL: " + msg) }
func (f *fakeLogger) With(fields ...logging.Field) logging.Logger {
	return f
}
func (f *fakeLogger) Sync() error { return nil }

func (f *fakeLogger) log(msg string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.messages = append(f.messages, msg)
}

func (f *fakeLogger) Messages() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.messages...)
}

func TestNewNginxAdapter(t *testing.T) {
	logger := &fakeLogger{}

	t.Run("ValidConfig", func(t *testing.T) {
		cfg := config.NginxConfig{
			Type:            "nginx",
			Name:            "test-nginx",
			Addresses:       []config.AddressConfig{{IP: "127.0.0.1", Port: 8080}},
			Protocol:        "http",
			Hostname:        "localhost",
			RequestPoolSize: 2,
			RequestTimeout:  5 * time.Second,
		}
		adapter, err := NewNginxAdapter(cfg, logger)
		assert.NoError(t, err)
		assert.NotNil(t, adapter)
		assert.Equal(t, 1, len(adapter.clients))
		assert.Equal(t, int64(cfg.RequestPoolSize), int64(cfg.GetRequestPoolSize()))
	})

	t.Run("InvalidType", func(t *testing.T) {
		cfg := config.NginxConfig{Type: "invalid"}
		adapter, err := NewNginxAdapter(cfg, logger)
		assert.Error(t, err)
		assert.Nil(t, adapter)
		assert.Contains(t, err.Error(), "invalid load balancer type")
	})
}

func TestSyncUpstream(t *testing.T) {
	var mu sync.Mutex
	requests := []string{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requests = append(requests, r.URL.String())
		mu.Unlock()

		if strings.Contains(r.URL.Path, "upstream_list") {
			// Return bulk upstream list
			fmt.Fprint(w, "test-upstream:\nserver 192.168.1.1:8080 weight=1;\n")
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	u, err := url.Parse(server.URL)
	require.NoError(t, err)
	host := u.Hostname()
	port, err := strconv.Atoi(u.Port())
	require.NoError(t, err)

	cfg := config.NginxConfig{
		Type:            "nginx",
		Name:            "test-nginx",
		Addresses:       []config.AddressConfig{{IP: host, Port: port}},
		Protocol:        "http",
		Hostname:        "localhost",
		ListAPI:         "/upstream_list",
		AddAPI:          "/upstream_add",
		RemoveAPI:       "/upstream_remove",
		RequestPoolSize: 2,
		RequestTimeout:  5 * time.Second,
	}

	logger := &fakeLogger{}
	adapter, err := NewNginxAdapter(cfg, logger)
	require.NoError(t, err)

	t.Run("SyncUpstream_AddAndUpdateAndRemove_WithIPv4AndIPv6", func(t *testing.T) {
		ctx := context.Background()
		upstream := model.Upstream{
			Name: "test-upstream",
			Backends: []model.Backend{
				// Keep 192.168.1.1:8080 but change weight from 1 to 2
				{IP: "192.168.1.1", Port: 8080, Weight: 2},
				// Add IPv6 backend
				{IP: "2001:db8::1", Port: 8081, Weight: 3},
				// Duplicate pod on same IPv6 node - should aggregate weights
				{IP: "[2001:db8::1]", Port: 8081, Weight: 1},
			},
		}

		err := adapter.SyncUpstream(ctx, upstream)
		assert.NoError(t, err)

		mu.Lock()
		reqs := append([]string(nil), requests...)
		mu.Unlock()

		assert.True(t, len(reqs) >= 2)
		// Check that IPv6 was formatted with brackets
		var foundIPv6Add bool
		for _, req := range reqs {
			if strings.Contains(req, "upstream_add") && strings.Contains(req, "%5B2001%3Adb8%3A%3A1%5D%3A8081") {
				foundIPv6Add = true
				// Aggregated weight should be 3 + 1 = 4
				assert.Contains(t, req, "weight=4")
			}
		}
		assert.True(t, foundIPv6Add, "Expected bracketed IPv6 in add request with aggregated weight")

		// Check structured logging
		logs := logger.Messages()
		var foundSyncLog bool
		for _, log := range logs {
			if strings.Contains(log, "Successfully synced backend to NGINX") {
				foundSyncLog = true
				break
			}
		}
		assert.True(t, foundSyncLog, "Expected structured logging on sync")
	})

	t.Run("SyncUpstream_EmptyBackendsDrainsAll", func(t *testing.T) {
		mu.Lock()
		requests = nil
		mu.Unlock()

		ctx := context.Background()
		upstream := model.Upstream{
			Name:     "test-upstream",
			Backends: []model.Backend{},
		}

		err := adapter.SyncUpstream(ctx, upstream)
		assert.NoError(t, err)

		mu.Lock()
		reqs := append([]string(nil), requests...)
		mu.Unlock()

		var foundRemove bool
		for _, req := range reqs {
			if strings.Contains(req, "upstream_remove") && strings.Contains(req, "192.168.1.1") && strings.Contains(req, "8080") {
				foundRemove = true
				break
			}
		}
		assert.True(t, foundRemove, "Expected upstream_remove to be called when draining backends")
	})

	t.Run("SyncUpstream_TTLCacheHit_Vs_MutationInvalidation", func(t *testing.T) {
		var listCalls int
		var cacheMu sync.Mutex

		cacheServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cacheMu.Lock()
			if strings.Contains(r.URL.Path, "upstream_list") {
				listCalls++
				fmt.Fprint(w, "cache-upstream:\nserver 10.10.10.1:8080 weight=2;\n")
				cacheMu.Unlock()
				return
			}
			cacheMu.Unlock()
			w.WriteHeader(http.StatusOK)
		}))
		defer cacheServer.Close()

		uCache, _ := url.Parse(cacheServer.URL)
		pCache, _ := strconv.Atoi(uCache.Port())

		cacheCfg := config.NginxConfig{
			Type:            "nginx",
			Name:            "cache-nginx",
			Addresses:       []config.AddressConfig{{IP: uCache.Hostname(), Port: pCache}},
			Protocol:        "http",
			Hostname:        "localhost",
			ListAPI:         "/upstream_list",
			AddAPI:          "/upstream_add",
			RemoveAPI:       "/upstream_remove",
			RequestPoolSize: 2,
			RequestTimeout:  5 * time.Second,
		}

		cAdapter, err := NewNginxAdapter(cacheCfg, &fakeLogger{})
		require.NoError(t, err)

		ctx := context.Background()
		up := model.Upstream{
			Name: "cache-upstream",
			Backends: []model.Backend{
				{IP: "10.10.10.1", Port: 8080, Weight: 2}, // exact match, no mutation
			},
		}

		// 1. First sync calls list_api
		err = cAdapter.SyncUpstream(ctx, up)
		require.NoError(t, err)
		cacheMu.Lock()
		assert.Equal(t, 1, listCalls)
		cacheMu.Unlock()

		// 2. Second sync immediately with no changes hits the 2-second cache (no list_api call)
		err = cAdapter.SyncUpstream(ctx, up)
		require.NoError(t, err)
		cacheMu.Lock()
		assert.Equal(t, 1, listCalls, "Expected cache hit; list_api should not be called again")
		cacheMu.Unlock()

		// 3. Third sync with mutation (change weight) invalidates cache
		upMutated := model.Upstream{
			Name: "cache-upstream",
			Backends: []model.Backend{
				{IP: "10.10.10.1", Port: 8080, Weight: 5},
			},
		}
		err = cAdapter.SyncUpstream(ctx, upMutated)
		require.NoError(t, err)

		// 4. Fourth sync must now re-query list_api because cache was invalidated
		err = cAdapter.SyncUpstream(ctx, upMutated)
		require.NoError(t, err)
		cacheMu.Lock()
		assert.Equal(t, 2, listCalls, "Expected cache miss after mutation invalidation")
		cacheMu.Unlock()
	})

	t.Run("SyncUpstream_SemaphoreConcurrencyLimiting", func(t *testing.T) {
		var activeRequests int32
		var maxObservedActive int32
		var semMu sync.Mutex

		semServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			semMu.Lock()
			activeRequests++
			if activeRequests > maxObservedActive {
				maxObservedActive = activeRequests
			}
			semMu.Unlock()

			// Artificially hold connection briefly to test concurrent ceiling
			time.Sleep(30 * time.Millisecond)

			semMu.Lock()
			activeRequests--
			semMu.Unlock()

			if strings.Contains(r.URL.Path, "upstream_list") {
				fmt.Fprint(w, "sem-upstream:\nserver 10.0.0.1:8080 weight=1;\n")
				return
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer semServer.Close()

		uSem, _ := url.Parse(semServer.URL)
		pSem, _ := strconv.Atoi(uSem.Port())

		poolLimit := 2
		semCfg := config.NginxConfig{
			Type:            "nginx",
			Name:            "sem-nginx",
			Addresses:       []config.AddressConfig{{IP: uSem.Hostname(), Port: pSem}},
			Protocol:        "http",
			Hostname:        "localhost",
			ListAPI:         "/upstream_list",
			AddAPI:          "/upstream_add",
			RemoveAPI:       "/upstream_remove",
			RequestPoolSize: poolLimit,
			RequestTimeout:  5 * time.Second,
		}

		semAdapter, err := NewNginxAdapter(semCfg, &fakeLogger{})
		require.NoError(t, err)

		var wg sync.WaitGroup
		ctx := context.Background()

		for i := 0; i < 6; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				_ = semAdapter.SyncUpstream(ctx, model.Upstream{
					Name: "sem-upstream",
					Backends: []model.Backend{
						{IP: "10.0.0.1", Port: 8080, Weight: idx + 1},
					},
				})
			}(i)
		}

		wg.Wait()

		semMu.Lock()
		assert.True(t, maxObservedActive <= int32(poolLimit),
			"Max active concurrent requests (%d) exceeded pool limit (%d)", maxObservedActive, poolLimit)
		semMu.Unlock()
	})

	t.Run("SyncUpstream_FlatFormatFallback", func(t *testing.T) {
		// Mock server returning flat format without upstream header line
		flatServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.Contains(r.URL.Path, "upstream_list") {
				fmt.Fprint(w, "server 192.168.1.1:8080 weight=1;\n")
				return
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer flatServer.Close()

		uFlat, _ := url.Parse(flatServer.URL)
		hFlat := uFlat.Hostname()
		pFlat, _ := strconv.Atoi(uFlat.Port())

		flatCfg := cfg
		flatCfg.Addresses = []config.AddressConfig{{IP: hFlat, Port: pFlat}}
		flatAdapter, err := NewNginxAdapter(flatCfg, logger)
		require.NoError(t, err)

		// Desired upstream matches existing flat backend exactly, should be no-op (no add or remove)
		ctx := context.Background()
		up := model.Upstream{
			Name: "any-upstream",
			Backends: []model.Backend{
				{IP: "192.168.1.1", Port: 8080, Weight: 1},
			},
		}

		err = flatAdapter.SyncUpstream(ctx, up)
		assert.NoError(t, err)
	})
}

func TestSyncAllUpstreams(t *testing.T) {
	var mu sync.Mutex
	var removeRequests []string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		if strings.Contains(r.URL.Path, "upstream_remove") {
			removeRequests = append(removeRequests, r.URL.String())
		}
		mu.Unlock()

		if strings.Contains(r.URL.Path, "upstream_list") {
			fmt.Fprint(w, "active-upstream:\nserver 10.0.0.1:8080 weight=1;\norphaned-upstream:\nserver 10.0.0.2:8080 weight=1;\n")
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	u, err := url.Parse(server.URL)
	require.NoError(t, err)
	host := u.Hostname()
	port, err := strconv.Atoi(u.Port())
	require.NoError(t, err)

	cfg := config.NginxConfig{
		Type:            "nginx",
		Name:            "test-nginx",
		Addresses:       []config.AddressConfig{{IP: host, Port: port}},
		Protocol:        "http",
		Hostname:        "localhost",
		ListAPI:         "/upstream_list",
		AddAPI:          "/upstream_add",
		RemoveAPI:       "/upstream_remove",
		RequestPoolSize: 2,
		RequestTimeout:  5 * time.Second,
	}

	logger := &fakeLogger{}
	adapter, err := NewNginxAdapter(cfg, logger)
	require.NoError(t, err)

	t.Run("SyncAllUpstreams_RemovesOrphanedAndLogs", func(t *testing.T) {
		ctx := context.Background()
		desired := map[string]model.Upstream{
			"active-upstream": {
				Name: "active-upstream",
				Backends: []model.Backend{
					{IP: "10.0.0.1", Port: 8080, Weight: 1},
				},
			},
		}

		err = adapter.SyncAllUpstreams(ctx, desired)
		assert.NoError(t, err)

		mu.Lock()
		defer mu.Unlock()
		assert.True(t, len(removeRequests) >= 1, "Expected orphaned-upstream backend to be removed")
	})

	t.Run("SyncAllUpstreams_ErrorAggregation", func(t *testing.T) {
		failServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.Contains(r.URL.Path, "upstream_list") {
				fmt.Fprint(w, "orphaned-upstream:\nserver 10.0.0.2:8080 weight=1;\n")
				return
			}
			http.Error(w, "removal forbidden", http.StatusForbidden)
		}))
		defer failServer.Close()

		uFail, _ := url.Parse(failServer.URL)
		hFail := uFail.Hostname()
		pFail, _ := strconv.Atoi(uFail.Port())

		failCfg := cfg
		failCfg.Addresses = []config.AddressConfig{{IP: hFail, Port: pFail}}
		failAdapter, err := NewNginxAdapter(failCfg, logger)
		require.NoError(t, err)

		err = failAdapter.SyncAllUpstreams(context.Background(), map[string]model.Upstream{})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "remove orphaned backend")
	})
}

func TestParseBulkNginxBackends(t *testing.T) {
	t.Run("MultiUpstreamResponse_WithVariedParameters", func(t *testing.T) {
		response := `
# Comment line to be ignored
// Another comment line
upstream1:
server 192.168.1.1:8080 max_fails=3 weight=2 fail_timeout=30s;
server 192.168.1.2:8081;

upstream2:
server 10.0.0.1:9090 weight=1;
server [2001:db8::1]:9091 backup weight=5 max_fails=1;
`
		parsed := parseBulkNginxBackends(response)
		require.Len(t, parsed, 2)

		assert.Len(t, parsed["upstream1"], 2)
		assert.Equal(t, "192.168.1.1", parsed["upstream1"][0].IP)
		assert.Equal(t, int32(8080), parsed["upstream1"][0].Port)
		assert.Equal(t, 2, parsed["upstream1"][0].Weight) // weight=2 preceded by max_fails=3

		assert.Equal(t, "192.168.1.2", parsed["upstream1"][1].IP)
		assert.Equal(t, int32(8081), parsed["upstream1"][1].Port)
		assert.Equal(t, 1, parsed["upstream1"][1].Weight) // default weight=1

		assert.Len(t, parsed["upstream2"], 2)
		assert.Equal(t, "10.0.0.1", parsed["upstream2"][0].IP)
		assert.Equal(t, int32(9090), parsed["upstream2"][0].Port)
		assert.Equal(t, 1, parsed["upstream2"][0].Weight)

		assert.Equal(t, "[2001:db8::1]", parsed["upstream2"][1].IP)
		assert.Equal(t, int32(9091), parsed["upstream2"][1].Port)
		assert.Equal(t, 5, parsed["upstream2"][1].Weight)
	})

	t.Run("SingleUpstream_FlatResponse", func(t *testing.T) {
		response := `
server 192.168.1.1:8080 weight=1;
server [fe80::1]:8082 weight=3;
`
		parsed := parseBulkNginxBackends(response)
		require.Len(t, parsed, 1)
		flatBackends := parsed[""]
		require.Len(t, flatBackends, 2)
		assert.Equal(t, "192.168.1.1", flatBackends[0].IP)
		assert.Equal(t, int32(8080), flatBackends[0].Port)
		assert.Equal(t, 1, flatBackends[0].Weight)

		assert.Equal(t, "[fe80::1]", flatBackends[1].IP)
		assert.Equal(t, int32(8082), flatBackends[1].Port)
		assert.Equal(t, 3, flatBackends[1].Weight)
	})

	t.Run("EmptyResponse", func(t *testing.T) {
		parsed := parseBulkNginxBackends("")
		assert.Empty(t, parsed)
	})

	t.Run("MalformedLines_SafelyIgnored", func(t *testing.T) {
		response := `
upstream_bad:
not a server line
server ;
server invalid_port:abc;
server 192.168.1.100:9000 weight=invalid;
server 192.168.1.101:9001 weight=10;
`
		parsed := parseBulkNginxBackends(response)
		require.Len(t, parsed, 1)
		require.Len(t, parsed["upstream_bad"], 2)
		// 192.168.1.100:9000 with invalid weight should default to 1
		assert.Equal(t, "192.168.1.100", parsed["upstream_bad"][0].IP)
		assert.Equal(t, int32(9000), parsed["upstream_bad"][0].Port)
		assert.Equal(t, 1, parsed["upstream_bad"][0].Weight)

		// 192.168.1.101:9001 with weight=10
		assert.Equal(t, "192.168.1.101", parsed["upstream_bad"][1].IP)
		assert.Equal(t, int32(9001), parsed["upstream_bad"][1].Port)
		assert.Equal(t, 10, parsed["upstream_bad"][1].Weight)
	})
}

func TestAddressHelpers(t *testing.T) {
	t.Run("formatServerAddr", func(t *testing.T) {
		assert.Equal(t, "192.168.1.1:8080", formatServerAddr("192.168.1.1", 8080))
		assert.Equal(t, "[2001:db8::1]:8080", formatServerAddr("2001:db8::1", 8080))
		assert.Equal(t, "[2001:db8::1]:8080", formatServerAddr("[2001:db8::1]", 8080))
	})

	t.Run("sameBackend", func(t *testing.T) {
		b1 := model.Backend{IP: "2001:db8::1", Port: 8080}
		b2 := model.Backend{IP: "[2001:db8::1]", Port: 8080}
		b3 := model.Backend{IP: "2001:db8::1", Port: 8081}
		assert.True(t, sameBackend(b1, b2))
		assert.False(t, sameBackend(b1, b3))
	})
}
