// internal/di/wire.go
package di

import (
	"fmt"
	certificate_adapter "mlb-controller/internal/adapters/certificate"
	config_adapter "mlb-controller/internal/adapters/config"
	"mlb-controller/internal/adapters/logging"
	metrics_adapter "mlb-controller/internal/adapters/metrics" // New import
	"mlb-controller/internal/application"
	config_ports "mlb-controller/internal/ports/config"
	logging_ports "mlb-controller/internal/ports/logging"
	metrics_ports "mlb-controller/internal/ports/metrics" // New import
)

// Container holds dependencies for the application.
type Container struct {
	Logger  logging_ports.Logger
	Loader  config_ports.Loader
	Metrics metrics_ports.Metrics // New field
	App     *application.App
}

// NewContainer creates and wires dependencies.
func NewContainer(configPath string) (*Container, error) {
	// Initialize bootstrap logger
	bootstrapLogger, err := logging.New(logging.LogConfig{Level: "info", Format: "json"})
	if err != nil {
		return nil, fmt.Errorf("failed to initialize bootstrap logger: %w", err)
	}

	// Create a new certificate loader instance
	certLoader := certificate_adapter.NewFileLoader()

	// Load config to determine production logger and metrics settings
	loader := config_adapter.NewViperLoader(configPath, certLoader)
	cfg, err := loader.Load()
	if err != nil {
		bootstrapLogger.Fatal("Failed to load config", logging_ports.Field{Key: "error", Value: err})
	}

	// Initialize production logger
	prodLoggerCfg := logging.LogConfig{Level: cfg.Log.Level, Format: cfg.Log.Format}
	prodLogger, err := logging.New(prodLoggerCfg)
	if err != nil {
		bootstrapLogger.Error("Failed to initialize production logger", logging_ports.Field{Key: "error", Value: err})
		prodLogger = bootstrapLogger
	}

	// Initialize metrics adapter
	metricsAdapter := metrics_adapter.NewPrometheusAdapter()

	// Create App
	app := application.NewApp(prodLogger, loader, metricsAdapter) // Pass metricsAdapter to NewApp

	return &Container{
		Logger:  prodLogger,
		Loader:  loader,
		Metrics: metricsAdapter,
		App:     app,
	}, nil
}
