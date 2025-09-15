package nginx

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"mlb-controller/internal/domain/config"

	"github.com/cenkalti/backoff/v4"
)

// NginxClient handles HTTP/HTTPS requests to an NGINX API address.
type NginxClient struct {
	client  *http.Client
	baseURL string // Protocl://IP:Port/path
	host    string // For HTTP Host header and SNI
}

// NewNginxClients creates a new NginxClient based on the provided AddressConfig.
func NewNginxClients(ngx config.NginxConfig) ([]*NginxClient, error) {
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: false,        // Always false as per requirement
			ServerName:         ngx.Hostname, // Set SNI for HTTPS
		},
	}

	// Handle HTTPS with client certificate if provided
	if ngx.Protocol == "https" && ngx.CertFile != "" && ngx.KeyFile != "" {
		cert, err := tls.X509KeyPair([]byte(ngx.CertFile), []byte(ngx.KeyFile))
		if err != nil {
			return nil, fmt.Errorf("failed to load client certificate: %w", err)
		}
		transport.TLSClientConfig.Certificates = []tls.Certificate{cert}
	}

	// Handle CA certificate for server verification
	if ngx.Protocol == "https" && ngx.CAFile != "" {
		caCertPool := x509.NewCertPool()
		if !caCertPool.AppendCertsFromPEM([]byte(ngx.CAFile)) {
			return nil, fmt.Errorf("failed to append CA certificate")
		}
		transport.TLSClientConfig.RootCAs = caCertPool
	}

	var clients []*NginxClient
	for _, addr := range ngx.GetAddresses() {
		baseURL := fmt.Sprintf("%s://%s:%d", ngx.Protocol, addr.IP, addr.Port)
		client := &http.Client{
			Transport: transport,
			Timeout:   ngx.RequestTimeout, // Configurable if needed
		}
		newngx := &NginxClient{
			client:  client,
			baseURL: baseURL,
			host:    ngx.Hostname,
		}
		clients = append(clients, newngx)
	}

	return clients, nil
}

// doRequest performs an HTTP request with the correct Host header and returns the response body.
func (c *NginxClient) doRequest(ctx context.Context, method, path string) ([]byte, error) {
	url := fmt.Sprintf("%s/%s", c.baseURL, strings.TrimPrefix(path, "/"))
	req, err := http.NewRequestWithContext(ctx, method, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set Host header if specified
	if c.host != "" {
		req.Host = c.host
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(body))
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	return bodyBytes, nil
}

// doRequestWithRetry performs an HTTP request with retries using exponential backoff.
func (c *NginxClient) doRequestWithRetry(ctx context.Context, method, path string) ([]byte, error) {
	var body []byte
	operation := func() error {
		var err error
		body, err = c.doRequest(ctx, method, path)
		if err != nil {
			return err
		}
		return nil
	}

	// Configure exponential backoff
	b := backoff.NewExponentialBackOff()
	b.MaxElapsedTime = 30 * time.Second // Total retry duration
	err := backoff.Retry(operation, backoff.WithContext(b, ctx))
	if err != nil {
		return nil, fmt.Errorf("failed after retries: %w", err)
	}

	return body, nil
}
