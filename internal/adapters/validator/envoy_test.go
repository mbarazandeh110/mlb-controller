package validator

import (
	"testing"
	"time"

	"mlb-controller/internal/domain/config"

	"github.com/stretchr/testify/assert"
)

func TestEnvoyValidator_Validate(t *testing.T) {
	tests := []struct {
		name        string
		config      *config.Config
		expectError bool
		errorMsg    string
	}{
		{
			name: "Valid Envoy Config",
			config: &config.Config{
				LoadBalancers: config.LoadBalancersConfig{
					Envoy: []config.EnvoyConfig{
						{
							Name:               "envoy1",
							UpstreamSyncPeriod: 5 * time.Second,
							RequestTimeout:     10 * time.Second,
						},
					},
				},
			},
			expectError: false,
		},
		{
			name: "Negative UpstreamSyncPeriod",
			config: &config.Config{
				LoadBalancers: config.LoadBalancersConfig{
					Envoy: []config.EnvoyConfig{
						{
							Name:               "envoy1",
							UpstreamSyncPeriod: -5 * time.Second,
							RequestTimeout:     10 * time.Second,
						},
					},
				},
			},
			expectError: true,
			errorMsg:    "envoy 'envoy1': upstream_sync_period must be non-negative",
		},
		{
			name: "Negative RequestTimeout",
			config: &config.Config{
				LoadBalancers: config.LoadBalancersConfig{
					Envoy: []config.EnvoyConfig{
						{
							Name:               "envoy1",
							UpstreamSyncPeriod: 5 * time.Second,
							RequestTimeout:     -10 * time.Second,
						},
					},
				},
			},
			expectError: true,
			errorMsg:    "envoy 'envoy1': request_timeout must be non-negative",
		},
		{
			name: "Empty Envoy Config List",
			config: &config.Config{
				LoadBalancers: config.LoadBalancersConfig{
					Envoy: []config.EnvoyConfig{},
				},
			},
			expectError: false,
		},
		{
			name: "Multiple Valid Envoy Configs",
			config: &config.Config{
				LoadBalancers: config.LoadBalancersConfig{
					Envoy: []config.EnvoyConfig{
						{
							Name:               "envoy1",
							UpstreamSyncPeriod: 5 * time.Second,
							RequestTimeout:     10 * time.Second,
						},
						{
							Name:               "envoy2",
							UpstreamSyncPeriod: 3 * time.Second,
							RequestTimeout:     20 * time.Second,
						},
					},
				},
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := &EnvoyValidator{}
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
