package main

import (
	"flag"
	"fmt"
	"os"

	"mlb-controller/internal/adapters/logging"
	"mlb-controller/internal/ports"

	"go.uber.org/zap"
)

func main() {
	flag.Parse()

	// Initialize logger with default configuration (info level, JSON format)
	defaultLoggerCfg := ports.LogConfig{
		Level:  "info",
		Format: "json",
	}
	var log ports.Logger
	log, err := logging.New(defaultLoggerCfg)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer func() {
		if err := log.Sync(); err != nil {
			log.Error("Failed to sync logger", zap.Error(err))
		}
	}()

	// Example log messages to verify logger functionality
	log.Info("Logger initialized successfully", zap.String("app", "mlb-controller"))
	log.Debug("This is a debug message", zap.Int("version", 1))
}
