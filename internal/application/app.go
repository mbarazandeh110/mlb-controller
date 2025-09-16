package application

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	domain "mlb-controller/internal/domain/config"
	config_ports "mlb-controller/internal/ports/config"
	logging_ports "mlb-controller/internal/ports/logging"
)

// App is the main application struct that orchestrates the controller logic.
type App struct {
	logger logging_ports.Logger
	loader config_ports.Loader
	config *domain.Config // از domain/config استفاده می‌کنیم
}

// NewApp creates a new App instance.
func NewApp(logger logging_ports.Logger, loader config_ports.Loader) *App {
	return &App{
		logger: logger,
		loader: loader,
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

	// TODO: Initialize Kubernetes client and leader election
	// This will be added in the next step

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

// Stop performs cleanup tasks (e.g., syncing logger).
func (a *App) Stop() error {
	if err := a.logger.Sync(); err != nil {
		a.logger.Error("Failed to sync logger", logging_ports.Field{Key: "error", Value: err})
		return err
	}
	return nil
}
