package logging

import (
	"fmt"
	"os"
	"strings"

	ports "mlb-controller/internal/ports/logging"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// LogConfig defines logger configuration.
type LogConfig struct {
	Level  string `yaml:"level"`  // debug, info, warn, error, fatal
	Format string `yaml:"format"` // json, console
}

// zapLogger implements ports.Logger using zap.
type zapLogger struct {
	zap *zap.Logger
}

// New creates a new zap-based logger based on the provided configuration.
func New(cfg LogConfig) (ports.Logger, error) {
	level, err := parseLevel(cfg.Level)
	if err != nil {
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

// toZapFields converts ports.Field to zap.Field.
func toZapFields(fields ...ports.Field) []zap.Field {
	zapFields := make([]zap.Field, len(fields))
	for i, f := range fields {
		switch v := f.Value.(type) {
		case string:
			zapFields[i] = zap.String(f.Key, v)
		case int:
			zapFields[i] = zap.Int(f.Key, v)
		case error:
			zapFields[i] = zap.Error(v)
		default:
			zapFields[i] = zap.Any(f.Key, v)
		}
	}
	return zapFields
}

func (l *zapLogger) Debug(msg string, fields ...ports.Field) {
	l.zap.Debug(msg, toZapFields(fields...)...)
}
func (l *zapLogger) Info(msg string, fields ...ports.Field) {
	l.zap.Info(msg, toZapFields(fields...)...)
}
func (l *zapLogger) Warn(msg string, fields ...ports.Field) {
	l.zap.Warn(msg, toZapFields(fields...)...)
}
func (l *zapLogger) Error(msg string, fields ...ports.Field) {
	l.zap.Error(msg, toZapFields(fields...)...)
}
func (l *zapLogger) Fatal(msg string, fields ...ports.Field) {
	l.zap.Fatal(msg, toZapFields(fields...)...)
}

func (l *zapLogger) With(fields ...ports.Field) ports.Logger {
	return &zapLogger{zap: l.zap.With(toZapFields(fields...)...)}
}

// Sync flushes any buffered log entries.
func (l *zapLogger) Sync() error {
	_ = l.zap.Sync() // Ignore errors for stdout
	return nil
}
