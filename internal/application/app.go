// internal/application/app.go
package application

import (
	"context"

	domain "mlb-controller/internal/domain/config"
	"mlb-controller/internal/ports/controller"
	"mlb-controller/internal/ports/leaderelection"
	logging_ports "mlb-controller/internal/ports/logging"
	metrics_ports "mlb-controller/internal/ports/metrics"

	"github.com/hashicorp/go-multierror"
)

// App is the main application struct that orchestrates the controller logic.
type App struct {
	logger         logging_ports.Logger
	config         *domain.Config
	metrics        metrics_ports.Metrics
	leaderElection leaderelection.LeaderElectionPort
	controller     controller.Controller
}

// NewApp creates a new App instance.
func NewApp(
	logger logging_ports.Logger,
	cfg *domain.Config,
	metrics metrics_ports.Metrics,
	leaderElection leaderelection.LeaderElectionPort,
	ctrl controller.Controller,
) *App {
	return &App{
		logger:         logger,
		config:         cfg,
		metrics:        metrics,
		leaderElection: leaderElection,
		controller:     ctrl,
	}
}

// Start runs the application, setting up leader election, metrics server, and waiting for context cancellation.
func (a *App) Start(ctx context.Context) error {
	a.logger.Info("Starting application...", logging_ports.Field{Key: "app", Value: "mlb-controller"})

	appCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Wire readiness checker
	if a.controller != nil && a.metrics != nil {
		a.metrics.SetReadyChecker(a.controller.HasSynced)
	}

	// Initialize leadership state on metrics
	if a.metrics != nil && a.leaderElection != nil {
		a.metrics.SetLeader(a.leaderElection.IsLeader())
	}

	// Start informers immediately so cache warms up and /readyz becomes ready across all replicas
	if a.controller != nil {
		go func() {
			if err := a.controller.StartInformers(appCtx); err != nil && appCtx.Err() == nil {
				a.logger.Error("Failed to start informers", logging_ports.Field{Key: "error", Value: err})
			}
		}()
	}

	// Start metrics server if enabled
	if a.config != nil && a.config.Metrics.Enabled {
		if err := a.metrics.Start(); err != nil {
			a.logger.Error("Failed to start metrics server", logging_ports.Field{Key: "error", Value: err})
			return err
		}
		a.logger.Info("Metrics server started", logging_ports.Field{Key: "port", Value: a.config.Metrics.Port}, logging_ports.Field{Key: "uri", Value: a.config.Metrics.URI})
	}

	// Set leader election callbacks
	a.leaderElection.SetCallbacks(leaderelection.Callbacks{
		OnStartedLeading: func(leadingCtx context.Context) {
			a.logger.Info("Started leading, initializing controller")
			if a.metrics != nil {
				a.metrics.SetLeader(true)
			}
			// Start controller for leader
			go func() {
				if err := a.controller.Start(leadingCtx); err != nil {
					a.logger.Error("Controller failed",
						logging_ports.Field{Key: "error", Value: err})
				}
			}()
		},
		OnStoppedLeading: func() {
			a.logger.Info("Stopped leading, shutting down controller")
			if a.metrics != nil {
				a.metrics.SetLeader(false)
			}
			// Trigger exit so kubelet restarts the pod
			cancel()
		},
	})

	// Start leader election
	go func() {
		if err := a.leaderElection.Run(appCtx); err != nil {
			a.logger.Error("Leader election failed", logging_ports.Field{Key: "error", Value: err})
			cancel()
		}
	}()

	// Block until context is cancelled (signal handled by main.go or leadership lost)
	<-appCtx.Done()
	a.logger.Info("Context canceled, stopping application")
	return nil
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
