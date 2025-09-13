// internal/adapters/loadbalancer/nginx/client.go
package nginx

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"mlb-controller/internal/domain/config"
)

// NginxClient handles HTTP/HTTPS requests to an NGINX API address.
type NginxClient struct {
	client  *http.Client
	baseURL string
	host    string // For HTTP Host header and SNI
}

// NewNginxClient creates a new NginxClient based on the provided AddressConfig.
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
			Timeout:   10 * time.Second, // Configurable if needed
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
func (c *NginxClient) doRequest(method, path string) ([]byte, error) {
	url := fmt.Sprintf("%s/%s", c.baseURL, strings.TrimPrefix(path, "/"))
	req, err := http.NewRequest(method, url, nil)
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
		return nil, fmt.Errorf("request failed with status: %d", resp.StatusCode)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	return bodyBytes, nil
}
