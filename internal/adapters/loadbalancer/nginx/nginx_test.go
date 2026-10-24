package nginx

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
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

func (f *fakeLogger) Debug(msg string, fields ...logging.Field) { f.log(msg) }
func (f *fakeLogger) Info(msg string, fields ...logging.Field)  { f.log(msg) }
func (f *fakeLogger) Warn(msg string, fields ...logging.Field)  { f.log(msg) }
func (f *fakeLogger) Error(msg string, fields ...logging.Field) { f.log(msg) }
func (f *fakeLogger) Fatal(msg string, fields ...logging.Field) { f.log(msg) }
func (f *fakeLogger) With(fields ...logging.Field) logging.Logger {
	return f
}
func (f *fakeLogger) Sync() error { return nil }

func (f *fakeLogger) log(msg string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.messages = append(f.messages, msg)
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

		if r.URL.Path == "/upstream_list" || r.URL.Path == "upstream_list" {
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

	t.Run("SyncUpstream_AddAndUpdateAndRemove", func(t *testing.T) {
		ctx := context.Background()
		upstream := model.Upstream{
			Name: "test-upstream",
			Backends: []model.Backend{
				// Keep 192.168.1.1:8080 but change weight from 1 to 2
				{IP: "192.168.1.1", Port: 8080, Weight: 2},
				// Add new backend 192.168.1.2:8081
				{IP: "192.168.1.2", Port: 8081, Weight: 1},
			},
		}

		err := adapter.SyncUpstream(ctx, upstream)
		assert.NoError(t, err)

		mu.Lock()
		defer mu.Unlock()
		assert.True(t, len(requests) >= 2)
	})

	t.Run("SyncUpstream_EmptyBackendsDrainsAll", func(t *testing.T) {
		ctx := context.Background()
		upstream := model.Upstream{
			Name:     "test-upstream",
			Backends: []model.Backend{},
		}

		err := adapter.SyncUpstream(ctx, upstream)
		assert.NoError(t, err)
	})
}

func TestSyncAllUpstreams(t *testing.T) {
	var mu sync.Mutex
	var removeRequests []string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		if r.URL.Path == "/upstream_remove" {
			removeRequests = append(removeRequests, r.URL.String())
		}
		mu.Unlock()

		if r.URL.Path == "/upstream_list" {
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
}

func TestParseBulkNginxBackends(t *testing.T) {
	t.Run("MultiUpstreamResponse", func(t *testing.T) {
		response := `
upstream1:
server 192.168.1.1:8080 weight=1;
server 192.168.1.2:8081 weight=2;

upstream2:
server 10.0.0.1:9090;
server [2001:db8::1]:9091 weight=5;
`
		parsed := parseBulkNginxBackends(response)
		require.Len(t, parsed, 2)
		assert.Len(t, parsed["upstream1"], 2)
		assert.Equal(t, "192.168.1.1", parsed["upstream1"][0].IP)
		assert.Equal(t, int32(8080), parsed["upstream1"][0].Port)
		assert.Equal(t, 1, parsed["upstream1"][0].Weight)
		assert.Equal(t, 2, parsed["upstream1"][1].Weight)

		assert.Len(t, parsed["upstream2"], 2)
		assert.Equal(t, "10.0.0.1", parsed["upstream2"][0].IP)
		assert.Equal(t, 1, parsed["upstream2"][0].Weight) // default weight
		assert.Equal(t, "[2001:db8::1]", parsed["upstream2"][1].IP)
		assert.Equal(t, 5, parsed["upstream2"][1].Weight)
	})

	t.Run("EmptyResponse", func(t *testing.T) {
		parsed := parseBulkNginxBackends("")
		assert.Empty(t, parsed)
	})
}
