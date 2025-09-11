package validator

import (
	"mlb-controller/internal/domain/config"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestEnvoyValidator_Validate(t *testing.T) {
	tests := []struct {
		name        string
		config      *config.Config
		expectError bool
		errorMsg    string
	}{
		{"Valid Envoy", &config.Config{
			LoadBalancers: config.LoadBalancersConfig{
				LoadBalancers: []config.LoadBalancerConfig{
					config.EnvoyConfig{
						Name:               "envoy1",
						UpstreamSyncPeriod: 10 * time.Second,
						RequestTimeout:     30 * time.Second,
					},
				},
			},
		}, false, ""},
		{"Negative Sync Period", &config.Config{
			LoadBalancers: config.LoadBalancersConfig{
				LoadBalancers: []config.LoadBalancerConfig{
					config.EnvoyConfig{
						Name:               "envoy1",
						UpstreamSyncPeriod: -10 * time.Second,
					},
				},
			},
		}, true, "upstream_sync_period must be non-negative"},
		{"Negative Request Timeout", &config.Config{
			LoadBalancers: config.LoadBalancersConfig{
				LoadBalancers: []config.LoadBalancerConfig{
					config.EnvoyConfig{
						Name:           "envoy1",
						RequestTimeout: -30 * time.Second,
					},
				},
			},
		}, true, "request_timeout must be non-negative"},
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
