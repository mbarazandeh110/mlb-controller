package nginx

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
	"time"

	"mlb-controller/internal/domain/config"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func generateTestCert(expired bool) ([]byte, []byte, error) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, err
	}

	notBefore := time.Now().Add(-2 * time.Hour)
	notAfter := time.Now().Add(2 * time.Hour)
	if expired {
		notBefore = time.Now().Add(-4 * time.Hour)
		notAfter = time.Now().Add(-2 * time.Hour)
	}

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: "localhost",
		},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              []string{"localhost"},
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		return nil, nil, err
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
	privBytes, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return nil, nil, err
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privBytes})

	return certPEM, keyPEM, nil
}

// TestNewNginxClientSet tests the creation of NginxClient instances.
func TestNewNginxClientSet(t *testing.T) {
	t.Run("ValidConfig_HTTP", func(t *testing.T) {
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

	t.Run("ValidConfig_HTTPS_WithCertAndCA", func(t *testing.T) {
		certPEM, keyPEM, err := generateTestCert(false)
		require.NoError(t, err)

		cfg := config.NginxConfig{
			Type:           "nginx",
			Addresses:      []config.AddressConfig{{IP: "127.0.0.1", Port: 8443}},
			Protocol:       "https",
			Hostname:       "localhost",
			RequestTimeout: 5 * time.Second,
			Cert:           certPEM,
			Key:            keyPEM,
			CA:             certPEM,
		}
		clients, err := NewNginxClientSet(cfg)
		assert.NoError(t, err)
		assert.Len(t, clients, 1)
	})

	t.Run("HTTPS_ExpiredCert_ReturnsError", func(t *testing.T) {
		certPEM, keyPEM, err := generateTestCert(true)
		require.NoError(t, err)

		cfg := config.NginxConfig{
			Type:           "nginx",
			Addresses:      []config.AddressConfig{{IP: "127.0.0.1", Port: 8443}},
			Protocol:       "https",
			Hostname:       "localhost",
			RequestTimeout: 5 * time.Second,
			Cert:           certPEM,
			Key:            keyPEM,
		}
		clients, err := NewNginxClientSet(cfg)
		assert.Error(t, err)
		assert.Nil(t, clients)
		assert.Contains(t, err.Error(), "has expired")
	})

	t.Run("HTTPS_InvalidKeyPair_ReturnsError", func(t *testing.T) {
		cfg := config.NginxConfig{
			Type:           "nginx",
			Addresses:      []config.AddressConfig{{IP: "127.0.0.1", Port: 8443}},
			Protocol:       "https",
			Hostname:       "localhost",
			RequestTimeout: 5 * time.Second,
			Cert:           []byte("invalid cert"),
			Key:            []byte("invalid key"),
		}
		clients, err := NewNginxClientSet(cfg)
		assert.Error(t, err)
		assert.Nil(t, clients)
		assert.Contains(t, err.Error(), "failed to load client certificate/key")
	})

	t.Run("HTTPS_InvalidCA_ReturnsError", func(t *testing.T) {
		cfg := config.NginxConfig{
			Type:           "nginx",
			Addresses:      []config.AddressConfig{{IP: "127.0.0.1", Port: 8443}},
			Protocol:       "https",
			Hostname:       "localhost",
			RequestTimeout: 5 * time.Second,
			CA:             []byte("invalid ca pem"),
		}
		clients, err := NewNginxClientSet(cfg)
		assert.Error(t, err)
		assert.Nil(t, clients)
		assert.Contains(t, err.Error(), "failed to append CA certificate to pool")
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
