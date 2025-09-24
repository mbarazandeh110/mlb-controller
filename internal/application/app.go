// internal/application/app.go
package application

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	domain "mlb-controller/internal/domain/config"
	config_ports "mlb-controller/internal/ports/config"
	"mlb-controller/internal/ports/controller"
	"mlb-controller/internal/ports/leaderelection"
	logging_ports "mlb-controller/internal/ports/logging"
	metrics_ports "mlb-controller/internal/ports/metrics"

	"github.com/hashicorp/go-multierror"
)

// App is the main application struct that orchestrates the controller logic.
type App struct {
	logger         logging_ports.Logger
	loader         config_ports.Loader
	metrics        metrics_ports.Metrics
	leaderElection leaderelection.LeaderElectionPort
	controller     controller.Controller
	config         *domain.Config
}

// NewApp creates a new App instance.
func NewApp(
	logger logging_ports.Logger,
	loader config_ports.Loader,
	metrics metrics_ports.Metrics,
	leaderElection leaderelection.LeaderElectionPort,
	ctrl controller.Controller,
) *App {
	return &App{
		logger:         logger,
		loader:         loader,
		metrics:        metrics,
		leaderElection: leaderElection,
		controller:     ctrl,
	}
}

// Start runs the application, loading config, setting up leader election, and starting metrics server.
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
		if err := a.metrics.Start(); err != nil {
			a.logger.Error("Failed to start metrics server", logging_ports.Field{Key: "error", Value: err})
			return err
		}
		a.logger.Info("Metrics server started", logging_ports.Field{Key: "port", Value: a.config.Metrics.Port}, logging_ports.Field{Key: "uri", Value: a.config.Metrics.URI})
	}

	// Set leader election callbacks
	a.leaderElection.SetCallbacks(leaderelection.Callbacks{
		OnStartedLeading: func(ctx context.Context) {
			a.logger.Info("Started leading, initializing controller")
			// Start controller for leader
			go func() {
				if err := a.controller.Start(ctx); err != nil {
					a.logger.Error("Controller failed",
						logging_ports.Field{Key: "error", Value: err})
				}
			}()
			// Initial sync when becoming leader (requirement 11)
			if err := a.controller.SyncUpstreams(ctx); err != nil {
				a.logger.Error("Initial upstream sync failed",
					logging_ports.Field{Key: "error", Value: err})
			}
		},
		OnStoppedLeading: func() {
			a.logger.Info("Stopped leading, shutting down controller")
			// No explicit controller cleanup needed, as context cancellation handles it
		},
	})

	// Start leader election
	go func() {
		if err := a.leaderElection.Run(ctx); err != nil {
			a.logger.Error("Leader election failed", logging_ports.Field{Key: "error", Value: err})
		}
	}()

	// Handle metrics proxy for non-leader instances
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
				if !a.leaderElection.IsLeader() && a.config.Metrics.Enabled {
					a.logger.Info("Non-leader instance, proxying metrics")
					if err := a.metrics.ProxyToLeader(ctx); err != nil {
						a.logger.Error("Failed to proxy metrics to leader",
							logging_ports.Field{Key: "error", Value: err})
					}
					// Wait before retrying to avoid flooding
					time.Sleep(5 * time.Second)
				} else {
					time.Sleep(1 * time.Second) // Check periodically
				}
			}
		}
	}()

	// Wait for shutdown signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	select {
	case <-sigCh:
		a.logger.Info("Received shutdown signal, stopping application")
	case <-ctx.Done():
		a.logger.Info("Context canceled, stopping application")
	}

	// Perform cleanup
	return a.Stop(ctx)
}

// Stop performs cleanup tasks (e.g., syncing logger and stopping metrics server).
func (a *App) Stop(ctx context.Context) error {
	var errs *multierror.Error

	// Sync logger
	if err := a.logger.Sync(); err != nil {
		errs = multierror.Append(errs, err)
	}

	// Stop metrics server if enabled
	if a.config != nil && a.config.Metrics.Enabled {
		if err := a.metrics.Stop(ctx); err != nil {
			a.logger.Error("Failed to stop metrics server", logging_ports.Field{Key: "error", Value: err})
			errs = multierror.Append(errs, err)
		}
	}

	return errs.ErrorOrNil()
}
