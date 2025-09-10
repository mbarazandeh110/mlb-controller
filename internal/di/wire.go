package di

import (
	"fmt"
	"mlb-controller/internal/adapters/config"
	"mlb-controller/internal/adapters/logging"
	"mlb-controller/internal/application"
	config_ports "mlb-controller/internal/ports/config"
	logging_ports "mlb-controller/internal/ports/logging"
)

// Container holds dependencies for the application.
type Container struct {
	Logger logging_ports.Logger
	Loader config_ports.Loader
	App    *application.App
}

// NewContainer creates and wires dependencies.
func NewContainer(configPath string) (*Container, error) {
	// Initialize bootstrap logger
	bootstrapLogger, err := logging.New(logging.LogConfig{Level: "info", Format: "json"})
	if err != nil {
		return nil, fmt.Errorf("failed to initialize bootstrap logger: %w", err)
	}

	// Load config to determine production logger settings
	loader := config.NewViperLoader(configPath)
	cfg, err := loader.Load()
	if err != nil {
		bootstrapLogger.Fatal("Failed to load config", logging_ports.Field{Key: "error", Value: err})
	}

	// Initialize production logger
	prodLoggerCfg := logging.LogConfig{Level: cfg.Log.Level, Format: cfg.Log.Format}
	prodLogger, err := logging.New(prodLoggerCfg)
	if err != nil {
		bootstrapLogger.Error("Failed to initialize production logger", logging_ports.Field{Key: "error", Value: err})
		prodLogger = bootstrapLogger
	}

	// Create App
	app := application.NewApp(prodLogger, loader)

	return &Container{
		Logger: prodLogger,
		Loader: loader,
		App:    app,
	}, nil
}
