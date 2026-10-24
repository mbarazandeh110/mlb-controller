package application

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	domain "mlb-controller/internal/domain/config"
	"mlb-controller/internal/domain/model"
	leaderelection_ports "mlb-controller/internal/ports/leaderelection"
	logging_ports "mlb-controller/internal/ports/logging"
	metrics_ports "mlb-controller/internal/ports/metrics"

	"github.com/stretchr/testify/assert"
)

// fakeMetrics implements metrics_ports.Metrics for testing.
type fakeMetrics struct {
	started  bool
	stopped  bool
	startErr error
	stopErr  error
}

func (f *fakeMetrics) Start() error {
	f.started = true
	return f.startErr
}

func (f *fakeMetrics) Stop(ctx context.Context) error {
	f.stopped = true
	return f.stopErr
}

func (f *fakeMetrics) ProxyToLeader(ctx context.Context) error {
	return nil
}

func (f *fakeMetrics) NewCounter(name, help string, labels []string) metrics_ports.Counter {
	return nil
}
func (f *fakeMetrics) NewGauge(name, help string, labels []string) metrics_ports.Gauge { return nil }
func (f *fakeMetrics) NewHistogram(name, help string, labels []string) metrics_ports.Histogram {
	return nil
}
func (f *fakeMetrics) UpdateBackendsTotal(upstream string, count float64)    {}
func (f *fakeMetrics) IncrementSyncErrors(upstream string)                   {}
func (f *fakeMetrics) ObserveSyncDuration(upstream string, duration float64) {}

// fakeLeaderElection implements leaderelection_ports.LeaderElectionPort for testing.
type fakeLeaderElection struct {
	callbacks leaderelection_ports.Callbacks
	runErr    error
}

func (f *fakeLeaderElection) SetCallbacks(callbacks leaderelection_ports.Callbacks) {
	f.callbacks = callbacks
}

func (f *fakeLeaderElection) Run(ctx context.Context) error {
	if f.callbacks.OnStartedLeading != nil {
		f.callbacks.OnStartedLeading(ctx)
	}
	<-ctx.Done()
	if f.callbacks.OnStoppedLeading != nil {
		f.callbacks.OnStoppedLeading()
	}
	return f.runErr
}

func (f *fakeLeaderElection) IsLeader() bool { return true }

func (f *fakeLeaderElection) GetLeaderAddr() string { return "127.0.0.1:9090" }

// fakeController implements controller.Controller for testing.
type fakeController struct {
	started       bool
	syncUpstreams bool
	startErr      error
	syncErr       error
}

func (f *fakeController) Start(ctx context.Context) error {
	f.started = true
	<-ctx.Done()
	return f.startErr
}

func (f *fakeController) SyncUpstreams(ctx context.Context) error {
	f.syncUpstreams = true
	return f.syncErr
}

func (f *fakeController) GetState() model.ControllerState {
	return model.ControllerState{}
}

// fakeLogger implements logging_ports.Logger for testing.
type fakeLogger struct {
	messages []string
	mu       sync.Mutex
}

func (f *fakeLogger) Debug(msg string, fields ...logging_ports.Field)         { f.log(msg) }
func (f *fakeLogger) Info(msg string, fields ...logging_ports.Field)          { f.log(msg) }
func (f *fakeLogger) Warn(msg string, fields ...logging_ports.Field)          { f.log(msg) }
func (f *fakeLogger) Error(msg string, fields ...logging_ports.Field)         { f.log(msg) }
func (f *fakeLogger) Fatal(msg string, fields ...logging_ports.Field)         { f.log(msg) }
func (f *fakeLogger) With(fields ...logging_ports.Field) logging_ports.Logger { return f }
func (f *fakeLogger) Sync() error                                             { return nil }

func (f *fakeLogger) log(msg string) {
	f.mu.Lock()
	f.messages = append(f.messages, msg)
	f.mu.Unlock()
}

func (f *fakeLogger) Messages() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.messages...)
}

func TestNewApp(t *testing.T) {
	logger := &fakeLogger{}
	cfg := &domain.Config{}
	metrics := &fakeMetrics{}
	leaderElection := &fakeLeaderElection{}
	ctrl := &fakeController{}

	app := NewApp(logger, cfg, metrics, leaderElection, ctrl)

	assert.NotNil(t, app)
	assert.Equal(t, logger, app.logger)
	assert.Equal(t, cfg, app.config)
	assert.Equal(t, metrics, app.metrics)
	assert.Equal(t, leaderElection, app.leaderElection)
	assert.Equal(t, ctrl, app.controller)
}

func TestAppStart_Success(t *testing.T) {
	logger := &fakeLogger{}
	cfg := &domain.Config{Metrics: domain.MetricsConfig{Enabled: true, Port: 9090, URI: "/metrics"}}
	metrics := &fakeMetrics{}
	leaderElection := &fakeLeaderElection{}
	ctrl := &fakeController{}

	app := NewApp(logger, cfg, metrics, leaderElection, ctrl)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := app.Start(ctx)

	assert.NoError(t, err)
	assert.True(t, metrics.started)
	assert.Contains(t, logger.Messages(), "Starting application...")
	assert.Contains(t, logger.Messages(), "Metrics server started")
	assert.Contains(t, logger.Messages(), "Context canceled, stopping application")
}

func TestAppStart_MetricsStartError(t *testing.T) {
	logger := &fakeLogger{}
	cfg := &domain.Config{Metrics: domain.MetricsConfig{Enabled: true, Port: 9090, URI: "/metrics"}}
	metrics := &fakeMetrics{startErr: errors.New("metrics start failed")}
	leaderElection := &fakeLeaderElection{}
	ctrl := &fakeController{}

	app := NewApp(logger, cfg, metrics, leaderElection, ctrl)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := app.Start(ctx)

	assert.Error(t, err)
	assert.Contains(t, logger.Messages(), "Failed to start metrics server")
	assert.True(t, metrics.started)
}

func TestAppStop_Success(t *testing.T) {
	logger := &fakeLogger{}
	cfg := &domain.Config{Metrics: domain.MetricsConfig{Enabled: true}}
	metrics := &fakeMetrics{}
	leaderElection := &fakeLeaderElection{}
	ctrl := &fakeController{}
	app := NewApp(logger, cfg, metrics, leaderElection, ctrl)

	ctx := context.Background()
	err := app.Stop(ctx)

	assert.NoError(t, err)
	assert.True(t, metrics.stopped)
}

func TestAppStop_MetricsStopError(t *testing.T) {
	logger := &fakeLogger{}
	cfg := &domain.Config{Metrics: domain.MetricsConfig{Enabled: true}}
	metrics := &fakeMetrics{stopErr: errors.New("metrics stop failed")}
	leaderElection := &fakeLeaderElection{}
	ctrl := &fakeController{}
	app := NewApp(logger, cfg, metrics, leaderElection, ctrl)

	ctx := context.Background()
	err := app.Stop(ctx)

	assert.Error(t, err)
	assert.True(t, metrics.stopped)
	assert.Contains(t, logger.Messages(), "Failed to stop metrics server")
}
