package logging

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"mlb-controller/internal/ports"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func TestNewLogger(t *testing.T) {
	tests := []struct {
		name        string
		config      Config
		wantLevel   zapcore.Level
		wantFormat  string
		expectError bool
	}{
		{
			name: "Valid Info JSON",
			config: Config{
				Level:  "info",
				Format: "json",
			},
			wantLevel:   zapcore.InfoLevel,
			wantFormat:  "json",
			expectError: false,
		},
		{
			name: "Valid Debug Console",
			config: Config{
				Level:  "debug",
				Format: "console",
			},
			wantLevel:   zapcore.DebugLevel,
			wantFormat:  "console",
			expectError: false,
		},
		{
			name: "Invalid Level",
			config: Config{
				Level:  "invalid",
				Format: "json",
			},
			wantLevel:   zapcore.InfoLevel,
			wantFormat:  "json",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger, err := New(tt.config)
			if tt.expectError {
				if err == nil {
					t.Error("Expected error for invalid config, got none")
				}
				return
			}
			if err != nil {
				t.Errorf("New() error = %v, want nil", err)
				return
			}

			// Verify logger is created and implements ports.Logger
			if _, ok := logger.(ports.Logger); !ok {
				t.Error("Logger does not implement ports.Logger")
			}
		})
	}
}

func TestLoggerMethods(t *testing.T) {
	// Use a buffer to capture log output
	var buf bytes.Buffer
	cfg := Config{Level: "debug", Format: "json"}
	logger, err := newTestLogger(cfg, &buf)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	// Test log methods
	tests := []struct {
		name      string
		logFunc   func(string, ...ports.Field)
		message   string
		fields    []ports.Field
		wantLevel string
	}{
		{
			name:      "Debug",
			logFunc:   logger.Debug,
			message:   "Debug message",
			fields:    []ports.Field{{Key: "key", Value: "value"}},
			wantLevel: "DEBUG",
		},
		{
			name:      "Info",
			logFunc:   logger.Info,
			message:   "Info message",
			fields:    []ports.Field{{Key: "count", Value: 42}},
			wantLevel: "INFO",
		},
		{
			name:      "Warn",
			logFunc:   logger.Warn,
			message:   "Warn message",
			fields:    []ports.Field{},
			wantLevel: "WARN",
		},
		{
			name:      "Error",
			logFunc:   logger.Error,
			message:   "Error message",
			fields:    []ports.Field{{Key: "error", Value: fmt.Errorf("test error")}},
			wantLevel: "ERROR",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf.Reset()
			tt.logFunc(tt.message, tt.fields...)

			// Parse JSON output
			var logEntry map[string]interface{}
			if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
				t.Fatalf("Failed to parse log output as JSON: %v", err)
			}

			// Verify log level and message
			if level, ok := logEntry["level"].(string); !ok || level != tt.wantLevel {
				t.Errorf("Expected level %q, got %q", tt.wantLevel, level)
			}
			if msg, ok := logEntry["msg"].(string); !ok || msg != tt.message {
				t.Errorf("Expected message %q, got %q", tt.message, msg)
			}
		})
	}
}

func TestLoggerWith(t *testing.T) {
	// Use a buffer to capture log output
	var buf bytes.Buffer
	cfg := Config{Level: "info", Format: "json"}
	logger, err := newTestLogger(cfg, &buf)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	// Create logger with additional fields
	newLogger := logger.With(ports.Field{Key: "context", Value: "test"}, ports.Field{Key: "id", Value: 1})

	// Log a message
	newLogger.Info("Test with fields")

	// Parse JSON output
	var logEntry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
		t.Fatalf("Failed to parse log output as JSON: %v", err)
	}

	// Verify additional fields
	if context, ok := logEntry["context"].(string); !ok || context != "test" {
		t.Errorf("Expected context field 'test', got %v", logEntry["context"])
	}
	if id, ok := logEntry["id"].(float64); !ok || id != 1 {
		t.Errorf("Expected id field 1, got %v", logEntry["id"])
	}
}

func TestLoggerSync(t *testing.T) {
	var buf bytes.Buffer
	cfg := Config{Level: "info", Format: "json"}
	logger, err := newTestLogger(cfg, &buf)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	// Sync should not return error for buffer
	if err := logger.Sync(); err != nil {
		t.Errorf("Sync() error = %v, want nil", err)
	}
}

// newTestLogger creates a logger with the given config and output buffer for testing.
func newTestLogger(cfg Config, buf *bytes.Buffer) (ports.Logger, error) {
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
		zapcore.AddSync(buf),
		level,
	)

	logger := zap.New(core, zap.AddCaller(), zap.AddCallerSkip(1), zap.AddStacktrace(zapcore.ErrorLevel))
	return &zapLogger{zap: logger}, nil
}
