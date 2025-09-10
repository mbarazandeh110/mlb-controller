package validator

import (
	"testing"

	"mlb-controller/internal/domain/config"

	"github.com/stretchr/testify/assert"
)

func TestLogValidator_Validate(t *testing.T) {
	tests := []struct {
		name        string
		config      *config.Config
		expectError bool
		errorMsg    string
	}{
		{
			name: "Valid Log Config",
			config: &config.Config{
				Log: config.LogConfig{
					Enabled: true,
					Level:   "info",
					Format:  "json",
				},
			},
			expectError: false,
		},
		{
			name: "Disabled Log Config",
			config: &config.Config{
				Log: config.LogConfig{
					Enabled: false,
					Level:   "invalid", // Ignored because Enabled=false
					Format:  "invalid", // Ignored because Enabled=false
				},
			},
			expectError: false,
		},
		{
			name: "Invalid Log Level",
			config: &config.Config{
				Log: config.LogConfig{
					Enabled: true,
					Level:   "invalid",
					Format:  "json",
				},
			},
			expectError: true,
			errorMsg:    "log.level must be one of: debug, info, warn, error, fatal",
		},
		{
			name: "Invalid Log Format",
			config: &config.Config{
				Log: config.LogConfig{
					Enabled: true,
					Level:   "info",
					Format:  "invalid",
				},
			},
			expectError: true,
			errorMsg:    "log.format must be one of: console, json",
		},
		{
			name: "Empty Log Level",
			config: &config.Config{
				Log: config.LogConfig{
					Enabled: true,
					Level:   "",
					Format:  "json",
				},
			},
			expectError: true,
			errorMsg:    "log.level must be one of: debug, info, warn, error, fatal",
		},
		{
			name: "Empty Log Format",
			config: &config.Config{
				Log: config.LogConfig{
					Enabled: true,
					Level:   "info",
					Format:  "",
				},
			},
			expectError: true,
			errorMsg:    "log.format must be one of: console, json",
		},
		{
			name:        "Empty Config",
			config:      &config.Config{},
			expectError: false,
		},
		{
			name: "Case Insensitive Log Level and Format",
			config: &config.Config{
				Log: config.LogConfig{
					Enabled: true,
					Level:   "DEBUG",
					Format:  "CONSOLE",
				},
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := &LogValidator{}
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
