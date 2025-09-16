// internal/adapters/metrics/prometheus.go
package metrics

import (
	"context"
	"fmt"
	"net/http"

	"mlb-controller/internal/ports/metrics"

	"github.com/prometheus/client_golang/prometheus"
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
	registry   *prometheus.Registry
	httpServer *http.Server
}

// NewPrometheusAdapter creates a new PrometheusAdapter instance.
func NewPrometheusAdapter() *PrometheusAdapter {
	reg := prometheus.NewRegistry()
	reg.MustRegister(prometheus.NewBuildInfoCollector())
	return &PrometheusAdapter{
		registry: reg,
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
func (p *PrometheusAdapter) Start(port int, uri string) error {
	addr := fmt.Sprintf(":%d", port)
	p.httpServer = &http.Server{
		Addr:    addr,
		Handler: promhttp.HandlerFor(p.registry, promhttp.HandlerOpts{}),
	}
	go func() {
		if err := p.httpServer.ListenAndServe(); err != http.ErrServerClosed {
			// In a real application, you would log this error.
			fmt.Printf("Metrics server failed: %v\n", err)
		}
	}()
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
