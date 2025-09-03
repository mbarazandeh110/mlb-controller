package ports

import "go.uber.org/zap"

// Logger defines the interface for structured logging (Port).
type Logger interface {
	Debug(msg string, fields ...zap.Field)
	Info(msg string, fields ...zap.Field)
	Warn(msg string, fields ...zap.Field)
	Error(msg string, fields ...zap.Field)
	Fatal(msg string, fields ...zap.Field)
	With(fields ...zap.Field) Logger
	Sync() error
}

// LogConfig defines logger configuration.
type LogConfig struct {
	Level  string `yaml:"level"`  // debug, info, warn, error, fatal
	Format string `yaml:"format"` // json, console
}
