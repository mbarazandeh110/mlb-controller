package validator

import (
	"testing"

	"mlb-controller/internal/domain/config"

	"github.com/stretchr/testify/assert"
)

func TestMetricsValidator_Validate(t *testing.T) {
	tests := []struct {
		name        string
		config      *config.Config
		expectError bool
		errorMsg    string
	}{
		{
			name: "Valid Metrics Config",
			config: &config.Config{
				Metrics: config.MetricsConfig{
					Enabled: true,
					Port:    9090,
					URI:     "/metrics",
				},
			},
			expectError: false,
		},
		{
			name: "Disabled Metrics",
			config: &config.Config{
				Metrics: config.MetricsConfig{
					Enabled: false,
					Port:    -1, // Invalid but ignored because Enabled=false
					URI:     "", // Invalid but ignored because Enabled=false
				},
			},
			expectError: false,
		},
		{
			name: "Invalid Port Negative",
			config: &config.Config{
				Metrics: config.MetricsConfig{
					Enabled: true,
					Port:    -1,
					URI:     "/metrics",
				},
			},
			expectError: true,
			errorMsg:    "metrics.port must be between 0 and 65535",
		},
		{
			name: "Invalid Port Too High",
			config: &config.Config{
				Metrics: config.MetricsConfig{
					Enabled: true,
					Port:    70000,
					URI:     "/metrics",
				},
			},
			expectError: true,
			errorMsg:    "metrics.port must be between 0 and 65535",
		},
		{
			name: "Empty URI",
			config: &config.Config{
				Metrics: config.MetricsConfig{
					Enabled: true,
					Port:    9090,
					URI:     "",
				},
			},
			expectError: true,
			errorMsg:    "metrics.uri is required when enabled",
		},
		{
			name: "Boundary Port Zero",
			config: &config.Config{
				Metrics: config.MetricsConfig{
					Enabled: true,
					Port:    0,
					URI:     "/metrics",
				},
			},
			expectError: false,
		},
		{
			name: "Boundary Port Max",
			config: &config.Config{
				Metrics: config.MetricsConfig{
					Enabled: true,
					Port:    65535,
					URI:     "/metrics",
				},
			},
			expectError: false,
		},
		{
			name:        "Empty Config",
			config:      &config.Config{},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := &MetricsValidator{}
			err := v.Validate(tt.config)

			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
