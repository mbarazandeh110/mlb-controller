// internal/adapters/loadbalancer/nginx/client.go
package nginx

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"mlb-controller/internal/domain/config"
)

type NginxClient struct {
	client  *http.Client
	baseURL string
	host    string
}

func NewNginxClientSet(ngx config.NginxConfig) ([]*NginxClient, error) {
	// بهینه‌سازی Transport برای جلوگیری از TIME_WAIT و Socket Exhaustion در ریکوئست‌های پرتعداد
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   5 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          200,
		MaxIdleConnsPerHost:   100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: false,
			ServerName:         ngx.Hostname,
		},
	}

	if ngx.Protocol == "https" && len(ngx.Key) > 0 {
		cert, err := tls.X509KeyPair(ngx.Cert, ngx.Key)
		if err != nil {
			return nil, fmt.Errorf("failed to load client certificate/key: %w", err)
		}
		transport.TLSClientConfig.Certificates = []tls.Certificate{cert}
		if len(cert.Certificate) > 0 {
			x509Cert, err := x509.ParseCertificate(cert.Certificate[0])
			if err != nil {
				return nil, fmt.Errorf("failed to parse client certificate: %w", err)
			}
			if time.Now().After(x509Cert.NotAfter) {
				return nil, fmt.Errorf("certificate %s has expired: NotAfter=%s", ngx.Hostname, x509Cert.NotAfter)
			}
		}
	}

	if ngx.Protocol == "https" && len(ngx.CA) > 0 {
		certPool := x509.NewCertPool()
		if !certPool.AppendCertsFromPEM(ngx.CA) {
			return nil, fmt.Errorf("failed to append CA certificate to pool")
		}
		transport.TLSClientConfig.RootCAs = certPool
	}

	var clients []*NginxClient
	for _, addr := range ngx.GetAddresses() {
		baseURL := fmt.Sprintf("%s://%s:%d", ngx.Protocol, addr.IP, addr.Port)
		client := &http.Client{
			Transport: transport,
			Timeout:   ngx.RequestTimeout,
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

// پیاده‌سازی مکانیزم Retry در سطح کلاینت HTTP برای مواجهه با Network Jitter یا لود موقتی
func (c *NginxClient) doRequest(ctx context.Context, method, path string) ([]byte, error) {
	url := fmt.Sprintf("%s/%s", c.baseURL, strings.TrimPrefix(path, "/"))
	var lastErr error
	var resp *http.Response

	maxRetries := 3
	for i := 0; i < maxRetries; i++ {
		req, err := http.NewRequestWithContext(ctx, method, url, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create request: %w", err)
		}

		if c.host != "" {
			req.Host = c.host
		}

		resp, err = c.client.Do(req)
		if err == nil && resp.StatusCode == 200 {
			defer resp.Body.Close()
			bodyBytes, err := io.ReadAll(resp.Body)
			if err != nil {
				return nil, fmt.Errorf("failed to read response body: %w", err)
			}
			return bodyBytes, nil
		}

		if resp != nil {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			lastErr = fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(body))

			// در خطاهای مربوط به سمت کلاینت (غیر از 429) تلاش مجدد بی‌فایده است
			if resp.StatusCode >= 400 && resp.StatusCode < 500 && resp.StatusCode != 429 {
				return nil, lastErr
			}
		} else {
			lastErr = err
		}

		// Exponential backoff
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Duration(100*(1<<i)) * time.Millisecond):
		}
	}

	return nil, fmt.Errorf("exhausted retries: %w", lastErr)
}
