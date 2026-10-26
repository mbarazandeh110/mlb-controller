// internal/adapters/metrics/prometheus_test.go
package metrics

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	domain "mlb-controller/internal/domain/config"
	leaderelection_ports "mlb-controller/internal/ports/leaderelection"
	"mlb-controller/internal/ports/logging"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeLogger struct {
	messages []string
	mu       sync.Mutex
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

type fakeLeaderElection struct {
	mu     sync.RWMutex
	leader bool
}

func (f *fakeLeaderElection) SetCallbacks(callbacks leaderelection_ports.Callbacks) {}
func (f *fakeLeaderElection) Run(ctx context.Context) error                         { <-ctx.Done(); return ctx.Err() }
func (f *fakeLeaderElection) IsLeader() bool {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.leader
}
func (f *fakeLeaderElection) SetLeader(val bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.leader = val
}
func (f *fakeLeaderElection) GetLeaderAddr(ctx context.Context) string { return "127.0.0.1:9090" }

func TestPrometheusAdapter_Healthz(t *testing.T) {
	adapter := NewPrometheusAdapter(&fakeLogger{}, nil, domain.MetricsConfig{
		Enabled: true,
		Port:    9090,
		URI:     "/metrics",
	})

	handler := adapter.Handler()

	req := httptest.NewRequest("GET", "/healthz", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "text/plain; charset=utf-8", rec.Header().Get("Content-Type"))
	assert.Equal(t, "ok", rec.Body.String())
}

func TestPrometheusAdapter_Readyz(t *testing.T) {
	adapter := NewPrometheusAdapter(&fakeLogger{}, nil, domain.MetricsConfig{
		Enabled: true,
		Port:    9090,
		URI:     "/metrics",
	})

	handler := adapter.Handler()

	t.Run("Default without readyChecker returns ok", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/readyz", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "ok", rec.Body.String())
	})

	t.Run("ReadyChecker returns false -> 503 Service Unavailable", func(t *testing.T) {
		var synced bool
		adapter.SetReadyChecker(func() bool { return synced })

		req := httptest.NewRequest("GET", "/readyz", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
		assert.Equal(t, "informers not synced", rec.Body.String())

		// Transition to synced -> 200 OK
		synced = true
		req2 := httptest.NewRequest("GET", "/readyz", nil)
		rec2 := httptest.NewRecorder()
		handler.ServeHTTP(rec2, req2)

		assert.Equal(t, http.StatusOK, rec2.Code)
		assert.Equal(t, "ok", rec2.Body.String())
	})
}

func TestPrometheusAdapter_MetricsEndpoint(t *testing.T) {
	le := &fakeLeaderElection{leader: true}
	adapter := NewPrometheusAdapter(&fakeLogger{}, le, domain.MetricsConfig{
		Enabled: true,
		Port:    9090,
		URI:     "/metrics",
	})

	adapter.UpdateBackendsTotal("test-upstream", 5)
	adapter.IncrementSyncErrors("test-upstream")
	adapter.ObserveSyncDuration("test-upstream", 0.042)

	handler := adapter.Handler()

	req := httptest.NewRequest("GET", "/metrics", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()

	assert.Contains(t, body, "mlb_backends_total{upstream=\"test-upstream\"} 5")
	assert.Contains(t, body, "mlb_sync_errors_total{upstream=\"test-upstream\"} 1")
	assert.Contains(t, body, "mlb_sync_duration_seconds_bucket")
	assert.Contains(t, body, "mlb_is_leader 1")
}

func TestPrometheusAdapter_LeaderStatus_DynamicGauge(t *testing.T) {
	le := &fakeLeaderElection{leader: false}
	adapter := NewPrometheusAdapter(&fakeLogger{}, le, domain.MetricsConfig{
		Enabled: true,
		Port:    9090,
		URI:     "/metrics",
	})

	handler := adapter.Handler()

	// 1. Initial state: standby (0)
	req := httptest.NewRequest("GET", "/metrics", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assert.Contains(t, rec.Body.String(), "mlb_is_leader 0")

	// 2. Becomes leader via leaderElection
	le.SetLeader(true)
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req)
	assert.Contains(t, rec2.Body.String(), "mlb_is_leader 1")

	// 3. Explicit SetLeader override
	adapter.SetLeader(false)
	le.SetLeader(false)
	rec3 := httptest.NewRecorder()
	handler.ServeHTTP(rec3, req)
	assert.Contains(t, rec3.Body.String(), "mlb_is_leader 0")

	adapter.SetLeader(true)
	rec4 := httptest.NewRecorder()
	handler.ServeHTTP(rec4, req)
	assert.Contains(t, rec4.Body.String(), "mlb_is_leader 1")
}

func TestPrometheusAdapter_CustomMetrics_Interfaces(t *testing.T) {
	adapter := NewPrometheusAdapter(&fakeLogger{}, nil, domain.MetricsConfig{
		Enabled: true,
		Port:    9090,
		URI:     "/metrics",
	})

	// Counter
	counter := adapter.NewCounter("test_counter", "help counter", nil)
	require.NotNil(t, counter)
	counter.Inc()
	counter.Add(3)

	// Gauge
	gauge := adapter.NewGauge("test_gauge", "help gauge", nil)
	require.NotNil(t, gauge)
	gauge.Set(10)
	gauge.Inc()
	gauge.Dec()
	gauge.Add(5)
	gauge.Sub(2)

	// Histogram
	hist := adapter.NewHistogram("test_hist", "help hist", nil)
	require.NotNil(t, hist)
	hist.Observe(1.5)

	handler := adapter.Handler()
	req := httptest.NewRequest("GET", "/metrics", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	assert.Contains(t, body, "test_counter 4")
	assert.Contains(t, body, "test_gauge 13")
	assert.Contains(t, body, "test_hist_count 1")
}

func TestPrometheusAdapter_CustomURI_And_NotFound(t *testing.T) {
	adapter := NewPrometheusAdapter(&fakeLogger{}, nil, domain.MetricsConfig{
		Enabled: true,
		Port:    9090,
		URI:     "/custom/metrics",
	})

	handler := adapter.Handler()

	// Metrics registered at /custom/metrics
	req := httptest.NewRequest("GET", "/custom/metrics", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)

	// Default /metrics not registered -> 404
	reqDefault := httptest.NewRequest("GET", "/metrics", nil)
	recDefault := httptest.NewRecorder()
	handler.ServeHTTP(recDefault, reqDefault)
	assert.Equal(t, http.StatusNotFound, recDefault.Code)

	// Unknown path -> 404
	reqUnknown := httptest.NewRequest("GET", "/unknown", nil)
	recUnknown := httptest.NewRecorder()
	handler.ServeHTTP(recUnknown, reqUnknown)
	assert.Equal(t, http.StatusNotFound, recUnknown.Code)
}

func TestPrometheusAdapter_ProxyToLeader_NoOp(t *testing.T) {
	adapter := NewPrometheusAdapter(&fakeLogger{}, nil, domain.MetricsConfig{
		Enabled: true,
		Port:    9090,
		URI:     "/metrics",
	})

	err := adapter.ProxyToLeader(context.Background())
	assert.NoError(t, err, "ProxyToLeader must be a safe no-op")
}

func TestPrometheusAdapter_StartAndStop(t *testing.T) {
	// Find a free TCP port dynamically
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()

	logger := &fakeLogger{}
	adapter := NewPrometheusAdapter(logger, nil, domain.MetricsConfig{
		Enabled: true,
		Port:    port,
		URI:     "/metrics",
	})

	err = adapter.Start()
	require.NoError(t, err)

	client := &http.Client{Timeout: 2 * time.Second}

	// Verify live HTTP server handles /healthz
	url := fmt.Sprintf("http://127.0.0.1:%d/healthz", port)
	require.Eventually(t, func() bool {
		resp, err := client.Get(url)
		if err != nil {
			return false
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		return resp.StatusCode == http.StatusOK && string(body) == "ok"
	}, 2*time.Second, 20*time.Millisecond)

	// Stop adapter
	stopCtx, stopCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer stopCancel()

	err = adapter.Stop(stopCtx)
	assert.NoError(t, err)

	// Stop when httpServer is nil should be no-op
	nilAdapter := &PrometheusAdapter{}
	assert.NoError(t, nilAdapter.Stop(context.Background()))
}
