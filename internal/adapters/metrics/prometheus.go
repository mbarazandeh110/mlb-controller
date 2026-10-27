// internal/adapters/metrics/prometheus.go
package metrics

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"

	domain "mlb-controller/internal/domain/config"
	"mlb-controller/internal/ports/leaderelection"
	"mlb-controller/internal/ports/logging"
	"mlb-controller/internal/ports/metrics"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// prometheusCounter implements the metrics.Counter interface.
type prometheusCounter struct {
	promCounter prometheus.Counter
}

func (c *prometheusCounter) Inc() {
	c.promCounter.Inc()
}

func (c *prometheusCounter) Add(delta float64) {
	c.promCounter.Add(delta)
}

// prometheusGauge implements the metrics.Gauge interface.
type prometheusGauge struct {
	promGauge prometheus.Gauge
}

func (g *prometheusGauge) Set(value float64) {
	g.promGauge.Set(value)
}

func (g *prometheusGauge) Inc() {
	g.promGauge.Inc()
}

func (g *prometheusGauge) Dec() {
	g.promGauge.Dec()
}

func (g *prometheusGauge) Add(delta float64) {
	g.promGauge.Add(delta)
}

func (g *prometheusGauge) Sub(delta float64) {
	g.promGauge.Sub(delta)
}

// prometheusHistogram implements the metrics.Histogram interface.
type prometheusHistogram struct {
	promHistogram prometheus.Histogram
}

func (h *prometheusHistogram) Observe(value float64) {
	h.promHistogram.Observe(value)
}

// PrometheusAdapter is the concrete adapter that implements the metrics.Metrics interface.
type PrometheusAdapter struct {
	registry       *prometheus.Registry
	httpServer     *http.Server
	logger         logging.Logger
	leaderElection leaderelection.LeaderElectionPort
	metricsConfig  domain.MetricsConfig

	readyChecker func() bool
	readyMutex   sync.RWMutex

	isLeader      bool
	isLeaderMutex sync.RWMutex

	// Metrics
	mlbBackendsTotal       *prometheus.GaugeVec
	mlbSyncErrorsTotal     *prometheus.CounterVec
	mlbSyncDurationSeconds *prometheus.HistogramVec
}

// NewPrometheusAdapter creates a new PrometheusAdapter instance.
func NewPrometheusAdapter(logger logging.Logger, leaderElection leaderelection.LeaderElectionPort, metricsConfig domain.MetricsConfig) *PrometheusAdapter {
	reg := prometheus.NewRegistry()
	reg.MustRegister(collectors.NewBuildInfoCollector())

	// Define metrics
	mlbBackendsTotal := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "mlb_backends_total",
			Help: "Total number of backends per upstream",
		},
		[]string{"upstream"},
	)
	mlbSyncErrorsTotal := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "mlb_sync_errors_total",
			Help: "Total number of sync errors",
		},
		[]string{"upstream"},
	)
	mlbSyncDurationSeconds := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "mlb_sync_duration_seconds",
			Help:    "Duration of sync operations in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"upstream"},
	)

	// Register core metrics
	reg.MustRegister(mlbBackendsTotal, mlbSyncErrorsTotal, mlbSyncDurationSeconds)

	p := &PrometheusAdapter{
		registry:               reg,
		logger:                 logger,
		leaderElection:         leaderElection,
		metricsConfig:          metricsConfig,
		mlbBackendsTotal:       mlbBackendsTotal,
		mlbSyncErrorsTotal:     mlbSyncErrorsTotal,
		mlbSyncDurationSeconds: mlbSyncDurationSeconds,
	}

	// Register mlb_is_leader dynamic gauge (1 = leader, 0 = standby)
	mlbIsLeader := prometheus.NewGaugeFunc(
		prometheus.GaugeOpts{
			Name: "mlb_is_leader",
			Help: "Leadership status of this controller replica (1 = leader, 0 = standby)",
		},
		func() float64 {
			p.isLeaderMutex.RLock()
			defer p.isLeaderMutex.RUnlock()
			if p.isLeader {
				return 1
			}
			if p.leaderElection != nil && p.leaderElection.IsLeader() {
				return 1
			}
			return 0
		},
	)
	reg.MustRegister(mlbIsLeader)

	return p
}

// SetReadyChecker sets a callback function to determine readiness (/readyz).
func (p *PrometheusAdapter) SetReadyChecker(checker func() bool) {
	p.readyMutex.Lock()
	defer p.readyMutex.Unlock()
	p.readyChecker = checker
}

// SetLeader updates the leadership status for the mlb_is_leader metric.
func (p *PrometheusAdapter) SetLeader(isLeader bool) {
	p.isLeaderMutex.Lock()
	defer p.isLeaderMutex.Unlock()
	p.isLeader = isLeader
}

func (p *PrometheusAdapter) NewCounter(name, help string, labels []string) metrics.Counter {
	opts := prometheus.CounterOpts{
		Name: name,
		Help: help,
	}
	counter := prometheus.NewCounter(opts)
	p.registry.MustRegister(counter)
	return &prometheusCounter{promCounter: counter}
}

func (p *PrometheusAdapter) NewGauge(name, help string, labels []string) metrics.Gauge {
	opts := prometheus.GaugeOpts{
		Name: name,
		Help: help,
	}
	gauge := prometheus.NewGauge(opts)
	p.registry.MustRegister(gauge)
	return &prometheusGauge{promGauge: gauge}
}

func (p *PrometheusAdapter) NewHistogram(name, help string, labels []string) metrics.Histogram {
	opts := prometheus.HistogramOpts{
		Name: name,
		Help: help,
	}
	histogram := prometheus.NewHistogram(opts)
	p.registry.MustRegister(histogram)
	return &prometheusHistogram{promHistogram: histogram}
}

// Handler returns the HTTP handler exposing the configured metrics URI, /healthz, and /readyz.
func (p *PrometheusAdapter) Handler() http.Handler {
	mux := http.NewServeMux()

	metricsURI := p.metricsConfig.URI
	if metricsURI == "" {
		metricsURI = "/metrics"
	}
	if !strings.HasPrefix(metricsURI, "/") {
		metricsURI = "/" + metricsURI
	}

	mux.Handle(metricsURI, promhttp.HandlerFor(p.registry, promhttp.HandlerOpts{}))

	// Liveness probe endpoint (/healthz)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	// Readiness probe endpoint (/readyz)
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		p.readyMutex.RLock()
		checker := p.readyChecker
		p.readyMutex.RUnlock()

		if checker != nil && !checker() {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte("informers not synced"))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	return mux
}

// Start launches the HTTP server to expose metrics, liveness (/healthz), and readiness (/readyz).
func (p *PrometheusAdapter) Start() error {
	addr := fmt.Sprintf(":%d", p.metricsConfig.Port)
	p.httpServer = &http.Server{
		Addr:    addr,
		Handler: p.Handler(),
	}
	go func() {
		if err := p.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			p.logger.Error("Metrics server failed",
				logging.Field{Key: "error", Value: err})
		}
	}()
	p.logger.Info("Metrics server started",
		logging.Field{Key: "port", Value: p.metricsConfig.Port},
		logging.Field{Key: "uri", Value: p.metricsConfig.URI})
	return nil
}

// Stop gracefully shuts down the HTTP server.
func (p *PrometheusAdapter) Stop(ctx context.Context) error {
	if p.httpServer == nil {
		return nil
	}

	return p.httpServer.Shutdown(ctx)
}

// ProxyToLeader is kept for interface compatibility; leadership state is exposed via mlb_is_leader.
func (p *PrometheusAdapter) ProxyToLeader(ctx context.Context) error {
	return nil
}

// UpdateBackendsTotal updates the number of backends for a given upstream.
func (p *PrometheusAdapter) UpdateBackendsTotal(upstream string, count float64) {
	p.mlbBackendsTotal.WithLabelValues(upstream).Set(count)
}

// IncrementSyncErrors increments the sync error counter for a given upstream.
func (p *PrometheusAdapter) IncrementSyncErrors(upstream string) {
	p.mlbSyncErrorsTotal.WithLabelValues(upstream).Inc()
}

// ObserveSyncDuration observes the sync duration for a given upstream.
func (p *PrometheusAdapter) ObserveSyncDuration(upstream string, duration float64) {
	p.mlbSyncDurationSeconds.WithLabelValues(upstream).Observe(duration)
}
