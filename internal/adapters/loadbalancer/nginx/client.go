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
)

// NginxClient handles HTTP/HTTPS requests to an NGINX API address.
type NginxClient struct {
	client  *http.Client
	baseURL string // Protocol://IP:Port/path
	host    string // For HTTP Host header and SNI
}

// NewNginxClientSet creates a new NginxClient based on the provided AddressConfig.
func NewNginxClientSet(ngx config.NginxConfig) ([]*NginxClient, error) {
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: false,        // Always false as per requirement
			ServerName:         ngx.Hostname, // Set SNI for HTTPS
		},
	}

	// Handle HTTPS with client certificate if provided
	if ngx.Protocol == "https" && len(ngx.Key) > 0 {
		cert, _ := tls.X509KeyPair(ngx.Cert, ngx.Key)
		transport.TLSClientConfig.Certificates = []tls.Certificate{cert}
		// Parse certificate to perform additional validations
		x509Cert, _ := x509.ParseCertificate(cert.Certificate[0])
		// Check certificate expiration
		if time.Now().After(x509Cert.NotAfter) {
			return nil, fmt.Errorf("certificate %s has expired: NotAfter=%s", ngx.Hostname, x509Cert.NotAfter)
		}
	}

	// Handle CA certificate for server verification
	if ngx.Protocol == "https" && len(ngx.CA) > 0 {
		transport.TLSClientConfig.RootCAs = x509.NewCertPool()
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

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(body))
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	return bodyBytes, nil
}
