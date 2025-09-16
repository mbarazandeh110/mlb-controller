// internal/application/app.go
package application

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	domain "mlb-controller/internal/domain/config"
	config_ports "mlb-controller/internal/ports/config"
	logging_ports "mlb-controller/internal/ports/logging"
	metrics_ports "mlb-controller/internal/ports/metrics" // New import
)

// App is the main application struct that orchestrates the controller logic.
type App struct {
	logger  logging_ports.Logger
	loader  config_ports.Loader
	metrics metrics_ports.Metrics // New field
	config  *domain.Config
}

// NewApp creates a new App instance.
func NewApp(logger logging_ports.Logger, loader config_ports.Loader, metrics metrics_ports.Metrics) *App {
	return &App{
		logger:  logger,
		loader:  loader,
		metrics: metrics,
	}
}

// Start runs the application, loading config and setting up leader election.
func (a *App) Start(ctx context.Context) error {
	// Load configuration
	cfg, err := a.loader.Load()
	if err != nil {
		a.logger.Error("Failed to load config", logging_ports.Field{Key: "error", Value: err})
		return err
	}

	a.config = cfg
	a.logger.Info("Configuration loaded successfully", logging_ports.Field{Key: "app", Value: "mlb-controller"})

	// Start metrics server if enabled
	if a.config.Metrics.Enabled {
		if err := a.metrics.Start(a.config.Metrics.Port, a.config.Metrics.URI); err != nil {
			a.logger.Error("Failed to start metrics server", logging_ports.Field{Key: "error", Value: err})
			return err
		}
		a.logger.Info("Metrics server started", logging_ports.Field{Key: "port", Value: a.config.Metrics.Port}, logging_ports.Field{Key: "uri", Value: a.config.Metrics.URI})
	}

	// Wait for shutdown signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	select {
	case <-sigCh:
		a.logger.Info("Received shutdown signal, stopping application")
	case <-ctx.Done():
		a.logger.Info("Context canceled, stopping application")
	}

	return nil
}

// Stop performs cleanup tasks (e.g., syncing logger and stopping metrics server).
// This function now accepts a context to manage the shutdown process.
func (a *App) Stop(ctx context.Context) error {
	var stopErr error
	if err := a.logger.Sync(); err != nil {
		stopErr = err
	}
	if a.config.Metrics.Enabled {
		if err := a.metrics.Stop(ctx); err != nil {
			a.logger.Error("Failed to stop metrics server", logging_ports.Field{Key: "error", Value: err})
			if stopErr == nil {
				stopErr = err
			}
		}
	}
	return stopErr
}
