package main

import (
	"context"
	"flag"
	"fmt"
	"os"

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
		_, _ = fmt.Fprintf(os.Stderr, "Failed to initialize container: %v\n", err)
		os.Exit(1)
	}
	defer container.App.Stop()

	// Start application
	ctx := context.Background()
	if err := container.App.Start(ctx); err != nil {
		container.Logger.Fatal("Application failed", logging_ports.Field{Key: "error", Value: err})
	}
}
