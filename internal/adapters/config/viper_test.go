package config

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"mlb-controller/internal/adapters/validator"
	"mlb-controller/internal/domain/config"
	config_ports "mlb-controller/internal/ports/config"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockValidator is a mock implementation of config_ports.Validator for testing.
type mockValidator struct {
	validateFunc func(cfg *config.Config) error
}

func (m *mockValidator) Validate(cfg *config.Config) error {
	if m.validateFunc != nil {
		return m.validateFunc(cfg)
	}
	return nil
}

func TestViperLoader_Load(t *testing.T) {
	// Create a temporary directory for test files
	tempDir, err := os.MkdirTemp("", "viper_test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	tests := []struct {
		name          string
		configContent string
		configPath    string
		validator     config_ports.Validator
		expected      *config.Config
		expectError   bool
		errorContains string
	}{
		{
			name: "Valid Config",
			configContent: `
global_upstream_sync_period: 5s
log:
  level: debug
  format: json
metrics:
  port: 8080
  uri: /metrics
loadbalancers:
  nginx:
    - name: nginx1
      list_api: /list
      add_api: /add
      remove_api: /remove
  envoy:
    - name: envoy1
`,
			configPath: filepath.Join(tempDir, "valid.yaml"),
			validator: &mockValidator{
				validateFunc: func(cfg *config.Config) error {
					return nil
				},
			},
			expected: &config.Config{
				GlobalUpstreamSyncPeriod: 5 * time.Second,
				LeaderElection: config.LeaderElectionConfig{
					LeaseDuration: 15 * time.Second,
					RenewDeadline: 10 * time.Second,
					RetryPeriod:   2 * time.Second,
				},
				Log: config.LogConfig{
					Level:  "debug",
					Format: "json",
				},
				Metrics: config.MetricsConfig{
					Port: 8080,
					URI:  "/metrics",
				},
				Kubernetes: config.KubernetesConfig{
					ResyncPeriod: 30 * time.Second,
				},
				LoadBalancers: config.LoadBalancersConfig{
					Nginx: []config.NginxConfig{{
						Name:               "nginx1",
						ListAPI:            "/list",
						AddAPI:             "/add",
						RemoveAPI:          "/remove",
						UpstreamSyncPeriod: 5 * time.Second,
						FailTimeout:        60 * time.Second,
						RequestTimeout:     30 * time.Second,
					}},
					Envoy: []config.EnvoyConfig{{
						Name:               "envoy1",
						UpstreamSyncPeriod: 5 * time.Second,
						RequestTimeout:     30 * time.Second,
					}},
				},
			},
			expectError: false,
		},
		{
			name:          "Non-existent Config File",
			configContent: "",
			configPath:    filepath.Join(tempDir, "nonexistent.yaml"),
			validator:     &mockValidator{},
			expected:      nil,
			expectError:   true,
			errorContains: "read config",
		},
		{
			name: "Invalid YAML",
			configContent: `
global_upstream_sync_period: 5s
log:
  level: debug
  format: json
metrics:
  port: 8080
  uri: /metrics
loadbalancers:
  nginx:
    - name: nginx1
      list_api: /list
      add_api: /add
      remove_api: /remove
    - name: nginx1  # Duplicate name
`,
			configPath:    filepath.Join(tempDir, "invalid.yaml"),
			validator:     validator.NewCompositeValidator(), // Use real validator
			expected:      nil,
			expectError:   true,
			errorContains: "loadbalancers.nginx.name 'nginx1' must be unique",
		},
		{
			name: "Validation Failure",
			configContent: `
global_upstream_sync_period: -5s  # Invalid negative value
`,
			configPath: filepath.Join(tempDir, "validation_failure.yaml"),
			validator: &mockValidator{
				validateFunc: func(cfg *config.Config) error {
					return fmt.Errorf("global_upstream_sync_period must be non-negative")
				},
			},
			expected:      nil,
			expectError:   true,
			errorContains: "validate config",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create config file if content is provided
			if tt.configContent != "" {
				err := os.WriteFile(tt.configPath, []byte(tt.configContent), 0644)
				require.NoError(t, err)
			}

			// Create ViperLoader with validator
			loader := &ViperLoader{
				path:      tt.configPath,
				validator: tt.validator,
			}

			// Test Load
			cfg, err := loader.Load()

			if tt.expectError {
				assert.Error(t, err)
				if err != nil { // Prevent nil pointer dereference
					assert.Contains(t, err.Error(), tt.errorContains)
				}
				assert.Nil(t, cfg)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, cfg)
			}
		})
	}
}
