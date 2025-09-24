// internal/adapters/metrics/prometheus.go
package metrics

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"time"

	"mlb-controller/internal/ports/leaderelection"
	"mlb-controller/internal/ports/logging"
	"mlb-controller/internal/ports/metrics"

	domain "mlb-controller/internal/domain/config"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/common/expfmt"
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
	metricsConfig  domain.MetricsConfig // Added for URI and Port
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

	// Register metrics
	reg.MustRegister(mlbBackendsTotal, mlbSyncErrorsTotal, mlbSyncDurationSeconds)

	return &PrometheusAdapter{
		registry:               reg,
		logger:                 logger,
		leaderElection:         leaderElection,
		metricsConfig:          metricsConfig,
		mlbBackendsTotal:       mlbBackendsTotal,
		mlbSyncErrorsTotal:     mlbSyncErrorsTotal,
		mlbSyncDurationSeconds: mlbSyncDurationSeconds,
	}
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

// Start launches the HTTP server to expose metrics.
func (p *PrometheusAdapter) Start() error {
	addr := fmt.Sprintf(":%d", p.metricsConfig.Port)
	p.httpServer = &http.Server{
		Addr:    addr,
		Handler: promhttp.HandlerFor(p.registry, promhttp.HandlerOpts{}),
	}
	go func() {
		if err := p.httpServer.ListenAndServe(); err != http.ErrServerClosed {
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

	// Use the provided context to gracefully shut down the server.
	// This will wait for active connections to finish.
	return p.httpServer.Shutdown(ctx)
}

// ProxyToLeader sends local metrics to the leader instance.
func (p *PrometheusAdapter) ProxyToLeader(ctx context.Context) error {
	leaderAddr := p.leaderElection.GetLeaderAddr()
	if leaderAddr == "" {
		return fmt.Errorf("leader address not available")
	}

	// Collect metrics from the local registry
	var buf bytes.Buffer
	enc := expfmt.NewEncoder(&buf, expfmt.NewFormat(expfmt.TypeTextPlain))
	families, err := p.registry.Gather()
	if err != nil {
		return fmt.Errorf("failed to gather metrics: %w", err)
	}
	for _, f := range families {
		if err := enc.Encode(f); err != nil {
			return fmt.Errorf("failed to encode metrics: %w", err)
		}
	}

	// Send metrics to the leader
	url := fmt.Sprintf("http://%s:%d/%s", leaderAddr, p.metricsConfig.Port, p.metricsConfig.URI)
	req, err := http.NewRequestWithContext(ctx, "POST", url, &buf)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", string(expfmt.FmtText))

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send metrics to leader: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to proxy metrics, status: %d", resp.StatusCode)
	}

	p.logger.Info("Successfully proxied metrics to leader",
		logging.Field{Key: "leader", Value: url})
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
