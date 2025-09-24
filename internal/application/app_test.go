package application

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"sync"
	"testing"
	"time"

	domain "mlb-controller/internal/domain/config"
	leaderelection_ports "mlb-controller/internal/ports/leaderelection"
	logging_ports "mlb-controller/internal/ports/logging"
	metrics_ports "mlb-controller/internal/ports/metrics"

	"github.com/stretchr/testify/assert"
)

// fakeLoader implements config_ports.Loader for testing.
type fakeLoader struct {
	config *domain.Config
	err    error
}

func (f *fakeLoader) Load() (*domain.Config, error) {
	return f.config, f.err
}

// fakeMetrics implements metrics_ports.Metrics for testing.
type fakeMetrics struct {
	started  bool
	stopped  bool
	startErr error
	stopErr  error
}

func (f *fakeMetrics) Start(port int, uri string) error {
	f.started = true
	return f.startErr
}

func (f *fakeMetrics) Stop(ctx context.Context) error {
	f.stopped = true
	return f.stopErr
}

func (f *fakeMetrics) NewCounter(name, help string, labels []string) metrics_ports.Counter {
	return nil
}
func (f *fakeMetrics) NewGauge(name, help string, labels []string) metrics_ports.Gauge { return nil }
func (f *fakeMetrics) NewHistogram(name, help string, labels []string) metrics_ports.Histogram {
	return nil
}

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
	loader := &fakeLoader{}
	metrics := &fakeMetrics{}
	leaderElection := &fakeLeaderElection{}

	app := NewApp(logger, loader, metrics, leaderElection)

	assert.NotNil(t, app)
	assert.Equal(t, logger, app.logger)
	assert.Equal(t, loader, app.loader)
	assert.Equal(t, metrics, app.metrics)
	assert.Equal(t, leaderElection, app.leaderElection)
	assert.Nil(t, app.config)
}

func TestAppStart_Success(t *testing.T) {
	logger := &fakeLogger{}
	loader := &fakeLoader{
		config: &domain.Config{Metrics: domain.MetricsConfig{Enabled: true, Port: 9090, URI: "/metrics"}},
	}
	metrics := &fakeMetrics{}
	leaderElection := &fakeLeaderElection{}
	var started bool
	leaderElection.SetCallbacks(leaderelection_ports.Callbacks{
		OnStartedLeading: func(ctx context.Context) { started = true },
		OnStoppedLeading: func() {},
	})

	app := NewApp(logger, loader, metrics, leaderElection)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Mock signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)

	go func() {
		time.Sleep(50 * time.Millisecond)
		sigCh <- os.Interrupt
	}()

	err := app.Start(ctx)

	assert.NoError(t, err)
	assert.True(t, metrics.started)
	assert.True(t, started)
	assert.Contains(t, logger.Messages(), "Configuration loaded successfully")
	assert.Contains(t, logger.Messages(), "Metrics server started")
	assert.Contains(t, logger.Messages(), "Received shutdown signal")
}

func TestAppStart_ConfigLoadError(t *testing.T) {
	logger := &fakeLogger{}
	loader := &fakeLoader{err: errors.New("config load failed")}
	metrics := &fakeMetrics{}
	leaderElection := &fakeLeaderElection{}

	app := NewApp(logger, loader, metrics, leaderElection)
	ctx := context.Background()

	err := app.Start(ctx)

	assert.Error(t, err)
	assert.Contains(t, logger.Messages(), "Failed to load config")
	assert.Equal(t, "", app.config)
	assert.False(t, metrics.started)
}

func TestAppStart_MetricsStartError(t *testing.T) {
	logger := &fakeLogger{}
	loader := &fakeLoader{
		config: &domain.Config{Metrics: domain.MetricsConfig{Enabled: true, Port: 9090, URI: "/metrics"}},
	}
	metrics := &fakeMetrics{startErr: errors.New("metrics start failed")}
	leaderElection := &fakeLeaderElection{}

	app := NewApp(logger, loader, metrics, leaderElection)
	ctx := context.Background()

	err := app.Start(ctx)

	assert.Error(t, err)
	assert.Contains(t, logger.Messages(), "Failed to start metrics server")
	assert.False(t, metrics.started)
}

func TestAppStop_Success(t *testing.T) {
	logger := &fakeLogger{}
	loader := &fakeLoader{config: &domain.Config{Metrics: domain.MetricsConfig{Enabled: true}}}
	metrics := &fakeMetrics{}
	leaderElection := &fakeLeaderElection{}
	app := NewApp(logger, loader, metrics, leaderElection)
	app.config = loader.config

	ctx := context.Background()
	err := app.Stop(ctx)

	assert.NoError(t, err)
	assert.True(t, metrics.stopped)
}

func TestAppStop_MetricsStopError(t *testing.T) {
	logger := &fakeLogger{}
	loader := &fakeLoader{config: &domain.Config{Metrics: domain.MetricsConfig{Enabled: true}}}
	metrics := &fakeMetrics{stopErr: errors.New("metrics stop failed")}
	leaderElection := &fakeLeaderElection{}
	app := NewApp(logger, loader, metrics, leaderElection)
	app.config = loader.config

	ctx := context.Background()
	err := app.Stop(ctx)

	assert.Error(t, err)
	assert.True(t, metrics.stopped)
	assert.Contains(t, logger.Messages(), "Failed to stop metrics server")
}
