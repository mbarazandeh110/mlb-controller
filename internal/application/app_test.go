package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"mlb-controller/internal/domain/config"
	config_ports "mlb-controller/internal/ports/config"
	"mlb-controller/internal/ports/logging"

	"github.com/stretchr/testify/assert"
)

type mockLogger struct {
	logs []string
}

func (m *mockLogger) Debug(msg string, fields ...logging.Field)   {}
func (m *mockLogger) Info(msg string, fields ...logging.Field)    { m.logs = append(m.logs, msg) }
func (m *mockLogger) Warn(msg string, fields ...logging.Field)    {}
func (m *mockLogger) Error(msg string, fields ...logging.Field)   {}
func (m *mockLogger) Fatal(msg string, fields ...logging.Field)   {}
func (m *mockLogger) With(fields ...logging.Field) logging.Logger { return m }
func (m *mockLogger) Sync() error                                 { return nil }

type mockLoader struct {
	config *config.Config
	err    error
}

func (m *mockLoader) Load() (*config.Config, error) { return m.config, m.err }

func TestApp_Start(t *testing.T) {
	tests := []struct {
		name        string
		loader      config_ports.Loader
		expectError bool
		expectedLog string
	}{
		{
			name: "Successful start",
			loader: &mockLoader{
				config: &config.Config{},
				err:    nil,
			},
			expectError: false,
			expectedLog: "Configuration loaded successfully",
		},
		{
			name: "Config load failure",
			loader: &mockLoader{
				config: nil,
				err:    errors.New("load failed"),
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := &mockLogger{}
			app := NewApp(logger, tt.loader)

			// Run Start in a goroutine to avoid blocking
			ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
			defer cancel()

			errCh := make(chan error)
			go func() {
				errCh <- app.Start(ctx)
			}()

			select {
			case err := <-errCh:
				if tt.expectError {
					assert.Error(t, err)
				} else {
					assert.NoError(t, err)
					assert.Contains(t, logger.logs, tt.expectedLog)
				}
			case <-ctx.Done():
				// Context canceled, which is expected
			}
		})
	}
}
