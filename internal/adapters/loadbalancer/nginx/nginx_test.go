package nginx

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
	"time"

	"mlb-controller/internal/domain/config"
	"mlb-controller/internal/domain/model"

	"github.com/stretchr/testify/assert"
)

// TestNewNginxAdapter tests the creation of a new NginxAdapter.
func TestNewNginxAdapter(t *testing.T) {
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
		adapter, err := NewNginxAdapter(cfg)
		assert.NoError(t, err)
		assert.NotNil(t, adapter)
		assert.Equal(t, 1, len(adapter.clients))
		assert.Equal(t, int64(cfg.RequestPoolSize), int64(cfg.GetRequestPoolSize()))
	})

	t.Run("InvalidType", func(t *testing.T) {
		cfg := config.NginxConfig{Type: "invalid"}
		adapter, err := NewNginxAdapter(cfg)
		assert.Error(t, err)
		assert.Nil(t, adapter)
		assert.Contains(t, err.Error(), "invalid load balancer type")
	})
}

// TestListBackends tests the ListBackends method with concurrent requests.
func TestListBackends(t *testing.T) {
	// Mock server with a valid NGINX response
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "server 192.168.1.1:8080 weight=1;\nserver 192.168.1.2:8081 weight=2;")
	}))
	defer server.Close()

	// Parse server.URL to extract host and port
	u, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("Failed to parse server URL: %v", err)
	}
	host := u.Hostname()
	port, err := strconv.Atoi(u.Port())
	if err != nil {
		t.Fatalf("Failed to parse server port: %v", err)
	}

	cfg := config.NginxConfig{
		Type:            "nginx",
		Name:            "test-nginx",
		Addresses:       []config.AddressConfig{{IP: host, Port: port}},
		Protocol:        "http",
		Hostname:        "localhost",
		RequestPoolSize: 2,
		RequestTimeout:  5 * time.Second,
	}
	adapter, err := NewNginxAdapter(cfg)
	assert.NoError(t, err)

	t.Run("SuccessfulList", func(t *testing.T) {
		ctx := context.Background()
		backends, err := adapter.ListBackends(ctx, "test-upstream")
		assert.NoError(t, err)
		assert.NotEmpty(t, backends)
		clientBackends, ok := backends[adapter.clients[0].baseURL]
		assert.True(t, ok)
		assert.Equal(t, 2, len(clientBackends))
		assert.Equal(t, model.Backend{IP: "192.168.1.1", Port: 8080, Weight: 1}, clientBackends[0])
		assert.Equal(t, model.Backend{IP: "192.168.1.2", Port: 8081, Weight: 2}, clientBackends[1])
	})

	t.Run("ConcurrencyWithSemaphore", func(t *testing.T) {
		// Mock server with delay to test semaphore
		delayServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(100 * time.Millisecond) // Simulate delay
			fmt.Fprint(w, "server 192.168.1.1:8080 weight=1;")
		}))
		defer delayServer.Close()

		// Parse delayServer.URL
		u, err := url.Parse(delayServer.URL)
		if err != nil {
			t.Fatalf("Failed to parse delay server URL: %v", err)
		}
		host := u.Hostname()
		port, err := strconv.Atoi(u.Port())
		if err != nil {
			t.Fatalf("Failed to parse delay server port: %v", err)
		}

		cfg.Addresses = []config.AddressConfig{{IP: host, Port: port}, {IP: host, Port: port}}
		adapter, err := NewNginxAdapter(cfg)
		assert.NoError(t, err)

		ctx := context.Background()
		start := time.Now()
		backends, err := adapter.ListBackends(ctx, "test-upstream")
		duration := time.Since(start)
		assert.NoError(t, err)
		assert.NotEmpty(t, backends)
		// With RequestPoolSize=2, two clients should run concurrently, so duration should be ~100ms
		assert.LessOrEqual(t, duration, 150*time.Millisecond, "Expected concurrent execution")
	})

	t.Run("ContextCancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately
		backends, err := adapter.ListBackends(ctx, "test-upstream")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to acquire semaphore")
		assert.Empty(t, backends)
	})
}

// TestAddBackend tests the AddBackend method.
func TestAddBackend(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Mock existing backends
		if _, ok := r.URL.Query()["verbose"]; ok {
			fmt.Fprint(w, "server 192.168.1.1:8080 weight=1;")
			return
		}
		// Simulate successful add/update
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Parse server.URL
	u, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("Failed to parse server URL: %v", err)
	}
	host := u.Hostname()
	port, err := strconv.Atoi(u.Port())
	if err != nil {
		t.Fatalf("Failed to parse server port: %v", err)
	}

	cfg := config.NginxConfig{
		Type:            "nginx",
		Addresses:       []config.AddressConfig{{IP: host, Port: port}},
		Protocol:        "http",
		Hostname:        "localhost",
		RequestPoolSize: 2,
		RequestTimeout:  5 * time.Second,
	}
	adapter, err := NewNginxAdapter(cfg)
	assert.NoError(t, err)

	t.Run("AddNewBackend", func(t *testing.T) {
		ctx := context.Background()
		backend := model.Backend{IP: "192.168.1.2", Port: 8081, Weight: 1}
		err := adapter.AddBackend(ctx, "test-upstream", backend)
		assert.NoError(t, err)
	})

	t.Run("UpdateExistingBackend", func(t *testing.T) {
		ctx := context.Background()
		backend := model.Backend{IP: "192.168.1.1", Port: 8080, Weight: 1}
		err := adapter.AddBackend(ctx, "test-upstream", backend)
		assert.NoError(t, err)
	})

	t.Run("RequestFailure", func(t *testing.T) {
		failServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}))
		defer failServer.Close()

		// Parse failServer.URL
		u, err := url.Parse(failServer.URL)
		if err != nil {
			t.Fatalf("Failed to parse fail server URL: %v", err)
		}
		host := u.Hostname()
		port, err := strconv.Atoi(u.Port())
		if err != nil {
			t.Fatalf("Failed to parse fail server port: %v", err)
		}

		cfg.Addresses = []config.AddressConfig{{IP: host, Port: port}}
		adapter, err := NewNginxAdapter(cfg)
		assert.NoError(t, err)

		ctx := context.Background()
		backend := model.Backend{IP: "192.168.1.2", Port: 8081, Weight: 1}
		err = adapter.AddBackend(ctx, "test-upstream", backend)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to parse backends")
	})
}

// TestRemoveBackend tests the RemoveBackend method.
func TestRemoveBackend(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := r.URL.Query()["verbose"]; ok {
			fmt.Fprint(w, "server 192.168.1.1:8080 weight=2;")
			return
		}
		// Simulate successful remove/update
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Parse server.URL
	u, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("Failed to parse server URL: %v", err)
	}
	host := u.Hostname()
	port, err := strconv.Atoi(u.Port())
	if err != nil {
		t.Fatalf("Failed to parse server port: %v", err)
	}

	cfg := config.NginxConfig{
		Type:            "nginx",
		Addresses:       []config.AddressConfig{{IP: host, Port: port}},
		Protocol:        "http",
		Hostname:        "localhost",
		RequestPoolSize: 2,
		RequestTimeout:  5 * time.Second,
	}
	adapter, err := NewNginxAdapter(cfg)
	assert.NoError(t, err)

	t.Run("RemoveBackend", func(t *testing.T) {
		ctx := context.Background()
		backend := model.Backend{IP: "192.168.1.1", Port: 8080, Weight: 2}
		err := adapter.RemoveBackend(ctx, "test-upstream", backend)
		assert.NoError(t, err)
	})

	t.Run("BackendNotFound", func(t *testing.T) {
		ctx := context.Background()
		backend := model.Backend{IP: "192.168.1.2", Port: 8081, Weight: 1}
		err := adapter.RemoveBackend(ctx, "test-upstream", backend)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "backend 192.168.1.2:8081 not found in upstream test-upstream")
	})
}

// TestSyncUpstream tests the SyncUpstream method.
func TestSyncUpstream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := r.URL.Query()["verbose"]; ok {
			fmt.Fprint(w, "server 192.168.1.1:8080 weight=1;")
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Parse server.URL
	u, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("Failed to parse server URL: %v", err)
	}
	host := u.Hostname()
	port, err := strconv.Atoi(u.Port())
	if err != nil {
		t.Fatalf("Failed to parse server port: %v", err)
	}

	cfg := config.NginxConfig{
		Type:            "nginx",
		Addresses:       []config.AddressConfig{{IP: host, Port: port}},
		Protocol:        "http",
		Hostname:        "localhost",
		RequestPoolSize: 2,
		RequestTimeout:  5 * time.Second,
	}
	adapter, err := NewNginxAdapter(cfg)
	assert.NoError(t, err)

	t.Run("SyncUpstream", func(t *testing.T) {
		ctx := context.Background()
		upstream := model.Upstream{
			Name: "test-upstream",
			Backends: []model.Backend{
				{IP: "192.168.1.1", Port: 8080, Weight: 1},
				{IP: "192.168.1.2", Port: 8081, Weight: 1},
			},
		}
		err := adapter.SyncUpstream(ctx, upstream)
		assert.NoError(t, err)
	})
}

// TestParseNginxBackends tests the parseNginxBackends function.
func TestParseNginxBackends(t *testing.T) {
	t.Run("ValidResponse", func(t *testing.T) {
		response := `
			server 192.168.1.1:8080 weight=1;
			server 192.168.1.2:8081 weight=2 max_fails=3 fail_timeout=30s;
		`
		backends, err := parseNginxBackends(response)
		assert.NoError(t, err)
		assert.Equal(t, 2, len(backends))
		assert.Equal(t, model.Backend{IP: "192.168.1.1", Port: 8080, Weight: 1}, backends[0])
		assert.Equal(t, model.Backend{IP: "192.168.1.2", Port: 8081, Weight: 2}, backends[1])
	})

	t.Run("InvalidResponse", func(t *testing.T) {
		response := `server invalid;`
		_, err := parseNginxBackends(response)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid backend format")
	})

	t.Run("EmptyResponse", func(t *testing.T) {
		response := ""
		backends, err := parseNginxBackends(response)
		assert.NoError(t, err)
		assert.Empty(t, backends)
	})
}
