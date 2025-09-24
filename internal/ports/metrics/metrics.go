// internal/ports/metrics/metrics.go
package metrics

import "context"

// Counter defines an interface for a metric that can be incremented.
type Counter interface {
	Inc()
	Add(delta float64)
}

// Gauge defines an interface for a metric that can be set to an arbitrary value.
type Gauge interface {
	Set(value float64)
	Inc()
	Dec()
	Add(delta float64)
	Sub(delta float64)
}

// Histogram defines an interface for a metric that samples observations and can be aggregated.
type Histogram interface {
	Observe(value float64)
}

// Metrics defines the interface for a metric collection system (Port).
type Metrics interface {
	NewCounter(name, help string, labels []string) Counter
	NewGauge(name, help string, labels []string) Gauge
	NewHistogram(name, help string, labels []string) Histogram
	Start() error
	// Stop gracefully shuts down the metrics server.
	Stop(ctx context.Context) error
	ProxyToLeader(ctx context.Context) error
	// Metrics-specific methods
	UpdateBackendsTotal(upstream string, count float64)
	IncrementSyncErrors(upstream string)
	ObserveSyncDuration(upstream string, duration float64)
}
