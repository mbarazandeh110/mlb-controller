package logging

import (
	"fmt"
	"os"
	"strings"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"mlb-controller/internal/ports"
)

// zapLogger implements ports.Logger using zap.
type zapLogger struct {
	zap *zap.Logger
}

// New creates a new zap-based logger based on the provided configuration.
func New(cfg ports.LogConfig) (ports.Logger, error) {
	level, err := parseLevel(cfg.Level)
	if err != nil {
		// Return error instead of writing to stderr for better error handling
		return nil, err
	}

	encoderCfg := zap.NewProductionEncoderConfig()
	encoderCfg.TimeKey = "ts"
	encoderCfg.EncodeTime = zapcore.ISO8601TimeEncoder
	encoderCfg.LevelKey = "level"
	encoderCfg.EncodeLevel = zapcore.CapitalLevelEncoder
	encoderCfg.MessageKey = "msg"
	encoderCfg.StacktraceKey = "stacktrace"

	var enc zapcore.Encoder
	switch strings.ToLower(cfg.Format) {
	case "console":
		enc = zapcore.NewConsoleEncoder(encoderCfg)
	default:
		enc = zapcore.NewJSONEncoder(encoderCfg)
	}

	core := zapcore.NewCore(
		enc,
		zapcore.AddSync(os.Stdout),
		level,
	)

	logger := zap.New(core, zap.AddCaller(), zap.AddCallerSkip(1), zap.AddStacktrace(zapcore.ErrorLevel))
	return &zapLogger{zap: logger}, nil
}

// parseLevel converts string level to zapcore.Level.
func parseLevel(level string) (zapcore.Level, error) {
	switch strings.ToLower(level) {
	case "debug":
		return zapcore.DebugLevel, nil
	case "info":
		return zapcore.InfoLevel, nil
	case "warn":
		return zapcore.WarnLevel, nil
	case "error":
		return zapcore.ErrorLevel, nil
	case "fatal":
		return zapcore.FatalLevel, nil
	default:
		return zapcore.InfoLevel, fmt.Errorf("invalid log level: %s, must be one of: debug, info, warn, error, fatal", level)
	}
}

func (l *zapLogger) Debug(msg string, fields ...zap.Field) { l.zap.Debug(msg, fields...) }
func (l *zapLogger) Info(msg string, fields ...zap.Field)  { l.zap.Info(msg, fields...) }
func (l *zapLogger) Warn(msg string, fields ...zap.Field)  { l.zap.Warn(msg, fields...) }
func (l *zapLogger) Error(msg string, fields ...zap.Field) { l.zap.Error(msg, fields...) }
func (l *zapLogger) Fatal(msg string, fields ...zap.Field) { l.zap.Fatal(msg, fields...) }
func (l *zapLogger) With(fields ...zap.Field) ports.Logger {
	return &zapLogger{zap: l.zap.With(fields...)}
}

// Sync flushes any buffered log entries.
func (l *zapLogger) Sync() error {
	_ = l.zap.Sync() // Ignore errors for stdout
	return nil
}
