package main

import (
	"context"
	"flag"

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
		container.Logger.Fatal("Failed to initialize container", logging_ports.Field{Key: "error", Value: err})
	}
	defer container.App.Stop()

	// Start application
	ctx := context.Background()
	if err := container.App.Start(ctx); err != nil {
		container.Logger.Fatal("Application failed", logging_ports.Field{Key: "error", Value: err})
	}
}
