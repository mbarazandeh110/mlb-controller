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
	"github.com/stretchr/testify/require"
)

// fakeMetrics implements metrics_ports.Metrics for testing.
type fakeMetrics struct {
	mu           sync.RWMutex
	started      bool
	stopped      bool
	proxied      bool
	startErr     error
	stopErr      error
	proxyErr     error
	readyChecker func() bool
	isLeader     bool
}

func (f *fakeMetrics) Start() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.started = true
	return f.startErr
}

func (f *fakeMetrics) Stop(ctx context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stopped = true
	return f.stopErr
}

func (f *fakeMetrics) ProxyToLeader(ctx context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.proxied = true
	return f.proxyErr
}

func (f *fakeMetrics) SetReadyChecker(checker func() bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.readyChecker = checker
}

func (f *fakeMetrics) SetLeader(isLeader bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.isLeader = isLeader
}

func (f *fakeMetrics) GetLeader() bool {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.isLeader
}

func (f *fakeMetrics) HasReadyChecker() bool {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.readyChecker != nil
}

func (f *fakeMetrics) IsStarted() bool {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.started
}

func (f *fakeMetrics) IsStopped() bool {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.stopped
}

func (f *fakeMetrics) IsProxied() bool {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.proxied
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
	mu        sync.RWMutex
	callbacks leaderelection_ports.Callbacks
	runErr    error
	leader    bool
}

func newFakeLeaderElection(isLeader bool) *fakeLeaderElection {
	return &fakeLeaderElection{
		leader: isLeader,
	}
}

func (f *fakeLeaderElection) SetCallbacks(callbacks leaderelection_ports.Callbacks) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.callbacks = callbacks
}

func (f *fakeLeaderElection) Run(ctx context.Context) error {
	f.mu.RLock()
	cb := f.callbacks
	runErr := f.runErr
	isLeader := f.leader
	f.mu.RUnlock()

	if isLeader && cb.OnStartedLeading != nil {
		cb.OnStartedLeading(ctx)
	}
	<-ctx.Done()
	if isLeader && cb.OnStoppedLeading != nil {
		cb.OnStoppedLeading()
	}
	return runErr
}

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

func (f *fakeLeaderElection) GetLeaderAddr() string { return "127.0.0.1:9090" }

// fakeController implements controller.Controller for testing.
type fakeController struct {
	mu            sync.RWMutex
	started       bool
	syncUpstreams bool
	startErr      error
	syncErr       error
}

func (f *fakeController) Start(ctx context.Context) error {
	f.mu.Lock()
	f.started = true
	startErr := f.startErr
	syncErr := f.syncErr
	f.mu.Unlock()

	if syncErr != nil {
		return syncErr
	}
	_ = f.SyncUpstreams(ctx)

	<-ctx.Done()
	return startErr
}

func (f *fakeController) StartInformers(ctx context.Context) error {
	return nil
}

func (f *fakeController) SyncUpstreams(ctx context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.syncUpstreams = true
	return f.syncErr
}

func (f *fakeController) IsStarted() bool {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.started
}

func (f *fakeController) HasSyncedUpstreams() bool {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.syncUpstreams
}

func (f *fakeController) GetState() model.ControllerState {
	return model.ControllerState{}
}

func (f *fakeController) HasSynced() bool {
	return true
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

func (f *fakeLogger) HasMessage(substring string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, m := range f.messages {
		if m == substring || (len(substring) > 0 && len(m) >= len(substring) && contains(m, substring)) {
			return true
		}
	}
	return false
}

func contains(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestNewApp(t *testing.T) {
	logger := &fakeLogger{}
	cfg := &domain.Config{}
	metrics := &fakeMetrics{}
	leaderElection := newFakeLeaderElection(true)
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
	leaderElection := newFakeLeaderElection(true)
	ctrl := &fakeController{}

	app := NewApp(logger, cfg, metrics, leaderElection, ctrl)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := app.Start(ctx)

	assert.NoError(t, err)
	assert.True(t, metrics.IsStarted())
	assert.True(t, logger.HasMessage("Starting application..."))
	assert.True(t, logger.HasMessage("Metrics server started"))
	assert.True(t, logger.HasMessage("Context canceled, stopping application"))
}

func TestAppStart_MetricsStartError(t *testing.T) {
	logger := &fakeLogger{}
	cfg := &domain.Config{Metrics: domain.MetricsConfig{Enabled: true, Port: 9090, URI: "/metrics"}}
	metrics := &fakeMetrics{startErr: errors.New("metrics start failed")}
	leaderElection := newFakeLeaderElection(true)
	ctrl := &fakeController{}

	app := NewApp(logger, cfg, metrics, leaderElection, ctrl)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := app.Start(ctx)

	assert.Error(t, err)
	assert.True(t, logger.HasMessage("Failed to start metrics server"))
	assert.True(t, metrics.IsStarted())
}

func TestAppStop_Success(t *testing.T) {
	logger := &fakeLogger{}
	cfg := &domain.Config{Metrics: domain.MetricsConfig{Enabled: true}}
	metrics := &fakeMetrics{}
	leaderElection := newFakeLeaderElection(true)
	ctrl := &fakeController{}
	app := NewApp(logger, cfg, metrics, leaderElection, ctrl)

	ctx := context.Background()
	err := app.Stop(ctx)

	assert.NoError(t, err)
	assert.True(t, metrics.IsStopped())
}

func TestAppStop_MetricsStopError(t *testing.T) {
	logger := &fakeLogger{}
	cfg := &domain.Config{Metrics: domain.MetricsConfig{Enabled: true}}
	metrics := &fakeMetrics{stopErr: errors.New("metrics stop failed")}
	leaderElection := newFakeLeaderElection(true)
	ctrl := &fakeController{}
	app := NewApp(logger, cfg, metrics, leaderElection, ctrl)

	ctx := context.Background()
	err := app.Stop(ctx)

	assert.Error(t, err)
	assert.True(t, metrics.IsStopped())
	assert.True(t, logger.HasMessage("Failed to stop metrics server"))
}

func TestApp_LeaderElection_Callbacks_OnStartedAndStoppedLeading(t *testing.T) {
	logger := &fakeLogger{}
	cfg := &domain.Config{Metrics: domain.MetricsConfig{Enabled: false}}
	metrics := &fakeMetrics{}
	leaderElection := newFakeLeaderElection(true)
	ctrl := &fakeController{}

	app := NewApp(logger, cfg, metrics, leaderElection, ctrl)
	ctx, cancel := context.WithCancel(context.Background())

	runDone := make(chan struct{})
	go func() {
		_ = app.Start(ctx)
		close(runDone)
	}()

	require.Eventually(t, func() bool {
		return ctrl.IsStarted() && ctrl.HasSyncedUpstreams() && metrics.GetLeader()
	}, 2*time.Second, 10*time.Millisecond)

	assert.True(t, logger.HasMessage("Started leading, initializing controller"))
	assert.True(t, metrics.GetLeader(), "metrics should report leader status as true")

	// Stop app to trigger OnStoppedLeading
	cancel()
	<-runDone

	require.Eventually(t, func() bool {
		return logger.HasMessage("Stopped leading, shutting down controller") && !metrics.GetLeader()
	}, 1*time.Second, 10*time.Millisecond)
	assert.False(t, metrics.GetLeader(), "metrics should report leader status as false after stopping")
}

func TestApp_LeaderElection_ControllerStartError(t *testing.T) {
	logger := &fakeLogger{}
	cfg := &domain.Config{Metrics: domain.MetricsConfig{Enabled: false}}
	metrics := &fakeMetrics{}
	leaderElection := newFakeLeaderElection(true)
	ctrl := &fakeController{startErr: errors.New("start failed")}

	app := NewApp(logger, cfg, metrics, leaderElection, ctrl)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_ = app.Start(ctx)

	require.Eventually(t, func() bool {
		return logger.HasMessage("Controller failed")
	}, 1*time.Second, 10*time.Millisecond)
}

func TestApp_LeaderElection_SyncUpstreamsError(t *testing.T) {
	logger := &fakeLogger{}
	cfg := &domain.Config{Metrics: domain.MetricsConfig{Enabled: false}}
	metrics := &fakeMetrics{}
	leaderElection := newFakeLeaderElection(true)
	ctrl := &fakeController{syncErr: errors.New("sync failed")}

	app := NewApp(logger, cfg, metrics, leaderElection, ctrl)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_ = app.Start(ctx)

	require.Eventually(t, func() bool {
		return logger.HasMessage("Controller failed")
	}, 1*time.Second, 10*time.Millisecond)
}

func TestApp_LeaderElection_RunError(t *testing.T) {
	logger := &fakeLogger{}
	cfg := &domain.Config{Metrics: domain.MetricsConfig{Enabled: false}}
	metrics := &fakeMetrics{}
	leaderElection := newFakeLeaderElection(true)
	leaderElection.runErr = errors.New("election failure")
	ctrl := &fakeController{}

	app := NewApp(logger, cfg, metrics, leaderElection, ctrl)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_ = app.Start(ctx)

	require.Eventually(t, func() bool {
		return logger.HasMessage("Leader election failed")
	}, 1*time.Second, 10*time.Millisecond)
}

func TestApp_ReadinessChecker_Wired(t *testing.T) {
	logger := &fakeLogger{}
	cfg := &domain.Config{Metrics: domain.MetricsConfig{Enabled: true, Port: 9090, URI: "/metrics"}}
	metrics := &fakeMetrics{}
	leaderElection := newFakeLeaderElection(true)
	ctrl := &fakeController{}

	app := NewApp(logger, cfg, metrics, leaderElection, ctrl)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_ = app.Start(ctx)

	assert.True(t, metrics.HasReadyChecker(), "App.Start must wire readiness checker on metrics")
}

func TestApp_NonLeader_MetricsReportsStandby(t *testing.T) {
	logger := &fakeLogger{}
	cfg := &domain.Config{Metrics: domain.MetricsConfig{Enabled: true, Port: 9090, URI: "/metrics"}}
	metrics := &fakeMetrics{}
	leaderElection := newFakeLeaderElection(false) // non-leader (standby)
	ctrl := &fakeController{}

	app := NewApp(logger, cfg, metrics, leaderElection, ctrl)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_ = app.Start(ctx)

	assert.False(t, metrics.GetLeader(), "Non-leader instance must report leadership status as false")
}
