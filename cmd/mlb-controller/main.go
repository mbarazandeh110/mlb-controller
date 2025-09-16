// cmd/mlb-controller/main.go
package main

import (
	"context"
	"flag"
	"os"
	"time"

	"mlb-controller/internal/adapters/logging"
	"mlb-controller/internal/di"
	logging_ports "mlb-controller/internal/ports/logging"
)

func main() {
	// Parse command-line arguments
	configPath := flag.String("config", "/etc/mlb-controller/config.yaml", "path to configuration file")
	flag.Parse()

	// Initialize dependencies
	container, err := di.NewContainer(*configPath)
	if err != nil {
		// Use a temporary logger if container failed
		tempLogger, _ := logging.New(logging.LogConfig{Level: "info", Format: "json"}) // Ignore err for simplicity
		tempLogger.Error("Failed to initialize container", logging_ports.Field{Key: "error", Value: err})
		tempLogger.Sync()
		os.Exit(1)
	}

	// This defer block now handles graceful shutdown with a timeout.
	// It creates a new context with a 5-second timeout for the Stop function.
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if stopErr := container.App.Stop(shutdownCtx); stopErr != nil {
			container.Logger.Error("Failed to stop app gracefully", logging_ports.Field{Key: "error", Value: stopErr})
		}
	}()

	// Start application
	ctx := context.Background()
	if err := container.App.Start(ctx); err != nil {
		container.Logger.Error("Application failed", logging_ports.Field{Key: "error", Value: err})
		os.Exit(1)
	}
}
