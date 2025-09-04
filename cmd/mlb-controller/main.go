package main

import (
	"flag"
	"fmt"
	"os"

	logging_adapters "mlb-controller/internal/adapters/logging"
	logging_ports "mlb-controller/internal/ports/logging"

	config_adapters "mlb-controller/internal/adapters/config"
	config_ports "mlb-controller/internal/ports/config"
)

func main() {
	// Parse command-line arguments
	configPath := flag.String("config", "/etc/mlb-controller/config.yaml", "path to configuration file")
	flag.Parse()

	// Initialize logger with default configuration (info level, JSON format)
	defaultLoggerCfg := logging_adapters.LogConfig{
		Level:  "info",
		Format: "json",
	}
	var log logging_ports.Logger
	log, err := logging_adapters.New(defaultLoggerCfg)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer func() {
		if err := log.Sync(); err != nil {
			log.Error("Failed to sync logger", logging_ports.Field{Key: "error", Value: err})
		}
	}()

	var loader config_ports.Loader
	loader = config_adapters.NewViperLoader(*configPath)
	// load config
	// cfg, err := loader.Load()
	_, err = loader.Load()
	if err != nil {
		log.Fatal("❌ failed to load config", logging_ports.Field{Key: "error", Value: err})
	}
	// fmt.Printf("%+v\n", cfg)

	// Example log messages to verify logger functionality
	log.Info("Logger initialized successfully", logging_ports.Field{Key: "app", Value: "mlb-controller"})
	log.Debug("This is a debug message", logging_ports.Field{Key: "version", Value: 1})
}
