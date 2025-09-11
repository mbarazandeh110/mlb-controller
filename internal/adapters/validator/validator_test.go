package validator

import (
	"testing"
	"time"

	"mlb-controller/internal/domain/config"

	"github.com/stretchr/testify/assert"
)

func TestCompositeValidator_Validate(t *testing.T) {
	tests := []struct {
		name        string
		config      *config.Config
		expectError bool
		errorMsg    string
	}{
		{
			name: "Valid Config",
			config: &config.Config{
				GlobalUpstreamSyncPeriod: 10 * time.Second,
				LeaderElection: config.LeaderElectionConfig{
					Enabled:        true,
					LeaseName:      "leader-lease",
					LeaseNamespace: "default",
					LeaseDuration:  15 * time.Second,
					RenewDeadline:  10 * time.Second,
					RetryPeriod:    2 * time.Second,
				},
				LoadBalancers: config.LoadBalancersConfig{
					Nginx: []config.NginxConfig{
						{
							Name:      "nginx1",
							ListAPI:   "/list",
							AddAPI:    "/add",
							RemoveAPI: "/remove",
							Addresses: []config.AddressConfig{
								{Protocol: "http", IP: "192.168.1.1", Port: 8080},
							},
						},
					},
					Envoy: []config.EnvoyConfig{
						{
							Name: "envoy1",
							Addresses: []config.AddressConfig{
								{Protocol: "https", IP: "10.0.0.1", Port: 443, Hostname: "example.com"},
							},
						},
					},
				},
			},
			expectError: false,
		},
		{
			name: "Invalid GlobalUpstreamSyncPeriod",
			config: &config.Config{
				GlobalUpstreamSyncPeriod: -5 * time.Second,
			},
			expectError: true,
			errorMsg:    "global_upstream_sync_period must be non-negative",
		},
		{
			name: "Missing LeaseName",
			config: &config.Config{
				GlobalUpstreamSyncPeriod: 10 * time.Second,
				LeaderElection: config.LeaderElectionConfig{
					Enabled:        true,
					LeaseName:      "",
					LeaseNamespace: "default",
					LeaseDuration:  15 * time.Second,
					RenewDeadline:  10 * time.Second,
					RetryPeriod:    2 * time.Second,
				},
			},
			expectError: true,
			errorMsg:    "leader_election.lease_name is required when enabled",
		},
		{
			name: "Duplicate LoadBalancer Names",
			config: &config.Config{
				GlobalUpstreamSyncPeriod: 10 * time.Second,
				LoadBalancers: config.LoadBalancersConfig{
					Nginx: []config.NginxConfig{
						{
							Name:      "lb1",
							ListAPI:   "/list",
							AddAPI:    "/add",
							RemoveAPI: "/remove",
							Addresses: []config.AddressConfig{
								{Protocol: "http", IP: "192.168.1.1", Port: 8080},
							},
						},
					},
					Envoy: []config.EnvoyConfig{
						{
							Name: "lb1",
							Addresses: []config.AddressConfig{
								{Protocol: "http", IP: "10.0.0.1", Port: 8080},
							},
						},
					},
				},
			},
			expectError: true,
			errorMsg:    "loadbalancers.envoy.name 'lb1' must be unique",
		},
		{
			name:        "Empty Config",
			config:      &config.Config{},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a new CompositeValidator with all validators
			v := NewCompositeValidator()
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

func TestGlobalConfigValidator_Validate(t *testing.T) {
	tests := []struct {
		name        string
		config      *config.Config
		expectError bool
		errorMsg    string
	}{
		{
			name: "Valid GlobalUpstreamSyncPeriod",
			config: &config.Config{
				GlobalUpstreamSyncPeriod: 10 * time.Second,
			},
			expectError: false,
		},
		{
			name: "Zero GlobalUpstreamSyncPeriod",
			config: &config.Config{
				GlobalUpstreamSyncPeriod: 0,
			},
			expectError: false,
		},
		{
			name: "Negative GlobalUpstreamSyncPeriod",
			config: &config.Config{
				GlobalUpstreamSyncPeriod: -5 * time.Second,
			},
			expectError: true,
			errorMsg:    "global_upstream_sync_period must be non-negative",
		},
		{
			name:        "Empty Config",
			config:      &config.Config{},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := &GlobalConfigValidator{}
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
