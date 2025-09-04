package main

import (
	"flag"
	"fmt"
	"os"

	logging_adapters "mlb-controller/internal/adapters/logging"
	logging_ports "mlb-controller/internal/ports/logging"

	config_adapters "mlb-controller/internal/adapters/config"
)

func main() {
	// Parse command-line arguments
	configPath := flag.String("config", "/etc/mlb-controller/config.yaml", "path to configuration file")
	flag.Parse()

	// Initialize logger with default configuration (info level, JSON format)
	var bootstrapLogger logging_ports.Logger
	bootstrapLogger, err := logging_adapters.New(logging_adapters.LogConfig{Level: "info", Format: "json"})
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer func() {
		if err := bootstrapLogger.Sync(); err != nil {
			bootstrapLogger.Error("Failed to sync logger", logging_ports.Field{Key: "error", Value: err})
		}
	}()

	// var loader config_ports.Loader
	loader := config_adapters.NewViperLoader(*configPath)
	// load config
	cfg, err := loader.Load()
	if err != nil {
		bootstrapLogger.Fatal("❌ failed to load config", logging_ports.Field{Key: "error", Value: err})
	}

	// Create production logger based on config
	prodLoggerCfg := logging_adapters.LogConfig{Level: cfg.Log.Level, Format: cfg.Log.Format}
	prodLogger, err := logging_adapters.New(prodLoggerCfg)
	if err != nil {
		bootstrapLogger.Error("Failed to initialize production logger", logging_ports.Field{Key: "error", Value: err})
		// Fallback to bootstrap
		prodLogger = bootstrapLogger
	}
	defer prodLogger.Sync()

	// Example log messages to verify logger functionality
	prodLogger.Info("Logger initialized successfully", logging_ports.Field{Key: "app", Value: "mlb-controller"})
	prodLogger.Debug("This is a debug message", logging_ports.Field{Key: "version", Value: 1})
}
