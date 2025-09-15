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

	"github.com/stretchr/testify/assert"
)

// TestNewNginxClientSet tests the creation of NginxClient instances.
func TestNewNginxClientSet(t *testing.T) {
	t.Run("ValidHTTPConfig", func(t *testing.T) {
		cfg := config.NginxConfig{
			Type:           "nginx",
			Addresses:      []config.AddressConfig{{IP: "127.0.0.1", Port: 8080}},
			Protocol:       "http",
			Hostname:       "localhost",
			RequestTimeout: 5 * time.Second,
		}
		clients, err := NewNginxClientSet(cfg)
		assert.NoError(t, err)
		assert.Equal(t, 1, len(clients))
		assert.Equal(t, "http://127.0.0.1:8080", clients[0].baseURL)
		assert.Equal(t, "localhost", clients[0].host)
		assert.Equal(t, 5*time.Second, clients[0].client.Timeout)
	})

	t.Run("ValidHTTPSConfig", func(t *testing.T) {
		cfg := config.NginxConfig{
			Type:           "nginx",
			Addresses:      []config.AddressConfig{{IP: "127.0.0.1", Port: 443}},
			Protocol:       "https",
			Hostname:       "localhost",
			CertFile:       "cert", // Mock cert
			KeyFile:        "key",  // Mock key
			CAFile:         "ca",   // Mock CA
			RequestTimeout: 5 * time.Second,
		}
		// Mock certificate loading (real certs not needed for test)
		_, err := NewNginxClientSet(cfg)
		assert.Error(t, err) // Expect error due to invalid cert/key
	})
}

// TestDoRequest tests the doRequest method of NginxClient.
func TestDoRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "localhost", r.Host)
		fmt.Fprint(w, "OK")
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
		Type:           "nginx",
		Addresses:      []config.AddressConfig{{IP: host, Port: port}},
		Protocol:       "http",
		Hostname:       "localhost",
		RequestTimeout: 5 * time.Second,
	}
	clients, err := NewNginxClientSet(cfg)
	assert.NoError(t, err)
	client := clients[0]

	t.Run("SuccessfulRequest", func(t *testing.T) {
		ctx := context.Background()
		body, err := client.doRequest(ctx, "GET", "/test")
		assert.NoError(t, err)
		assert.Equal(t, "OK", string(body))
	})

	t.Run("ServerError", func(t *testing.T) {
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
		clients, err := NewNginxClientSet(cfg)
		assert.NoError(t, err)
		client := clients[0]

		ctx := context.Background()
		_, err = client.doRequest(ctx, "GET", "/test")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "request failed with status 500")
	})

	t.Run("ContextTimeout", func(t *testing.T) {
		timeoutServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(100 * time.Millisecond)
			fmt.Fprint(w, "OK")
		}))
		defer timeoutServer.Close()

		// Parse timeoutServer.URL
		u, err := url.Parse(timeoutServer.URL)
		if err != nil {
			t.Fatalf("Failed to parse timeout server URL: %v", err)
		}
		host := u.Hostname()
		port, err := strconv.Atoi(u.Port())
		if err != nil {
			t.Fatalf("Failed to parse timeout server port: %v", err)
		}

		cfg.Addresses = []config.AddressConfig{{IP: host, Port: port}}
		cfg.RequestTimeout = 10 * time.Millisecond
		clients, err := NewNginxClientSet(cfg)
		assert.NoError(t, err)
		client := clients[0]

		ctx := context.Background()
		_, err = client.doRequest(ctx, "GET", "/test")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "context deadline exceeded")
	})
}
