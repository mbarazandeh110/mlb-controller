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

func TestLeaderElectionValidator_Validate(t *testing.T) {
	tests := []struct {
		name        string
		config      *config.Config
		expectError bool
		errorMsg    string
	}{
		{
			name: "Valid Leader Election Config",
			config: &config.Config{
				LeaderElection: config.LeaderElectionConfig{
					Enabled:        true,
					LeaseName:      "leader-lease",
					LeaseNamespace: "default",
					LeaseDuration:  15 * time.Second,
					RenewDeadline:  10 * time.Second,
					RetryPeriod:    2 * time.Second,
				},
			},
			expectError: false,
		},
		{
			name: "Disabled Leader Election",
			config: &config.Config{
				LeaderElection: config.LeaderElectionConfig{
					Enabled:        false,
					LeaseName:      "", // Ignored because Enabled=false
					LeaseNamespace: "",
					LeaseDuration:  0,
					RenewDeadline:  0,
					RetryPeriod:    0,
				},
			},
			expectError: false,
		},
		{
			name: "Missing LeaseName",
			config: &config.Config{
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
			name: "Missing LeaseNamespace",
			config: &config.Config{
				LeaderElection: config.LeaderElectionConfig{
					Enabled:        true,
					LeaseName:      "leader-lease",
					LeaseNamespace: "",
					LeaseDuration:  15 * time.Second,
					RenewDeadline:  10 * time.Second,
					RetryPeriod:    2 * time.Second,
				},
			},
			expectError: true,
			errorMsg:    "leader_election.lease_namespace is required when enabled",
		},
		{
			name: "Non-positive LeaseDuration",
			config: &config.Config{
				LeaderElection: config.LeaderElectionConfig{
					Enabled:        true,
					LeaseName:      "leader-lease",
					LeaseNamespace: "default",
					LeaseDuration:  0,
					RenewDeadline:  10 * time.Second,
					RetryPeriod:    2 * time.Second,
				},
			},
			expectError: true,
			errorMsg:    "leader_election.lease_duration must be positive",
		},
		{
			name: "Non-positive RenewDeadline",
			config: &config.Config{
				LeaderElection: config.LeaderElectionConfig{
					Enabled:        true,
					LeaseName:      "leader-lease",
					LeaseNamespace: "default",
					LeaseDuration:  15 * time.Second,
					RenewDeadline:  0,
					RetryPeriod:    2 * time.Second,
				},
			},
			expectError: true,
			errorMsg:    "leader_election.renew_deadline must be positive",
		},
		{
			name: "Non-positive RetryPeriod",
			config: &config.Config{
				LeaderElection: config.LeaderElectionConfig{
					Enabled:        true,
					LeaseName:      "leader-lease",
					LeaseNamespace: "default",
					LeaseDuration:  15 * time.Second,
					RenewDeadline:  10 * time.Second,
					RetryPeriod:    0,
				},
			},
			expectError: true,
			errorMsg:    "leader_election.retry_period must be positive",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := &LeaderElectionValidator{}
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

func TestLoadBalancersValidator_Validate(t *testing.T) {
	tests := []struct {
		name        string
		config      *config.Config
		expectError bool
		errorMsg    string
	}{
		{
			name: "Valid LoadBalancers Config",
			config: &config.Config{
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
			name: "Empty Nginx Name",
			config: &config.Config{
				LoadBalancers: config.LoadBalancersConfig{
					Nginx: []config.NginxConfig{
						{
							Name:      "",
							ListAPI:   "/list",
							AddAPI:    "/add",
							RemoveAPI: "/remove",
							Addresses: []config.AddressConfig{
								{Protocol: "http", IP: "192.168.1.1", Port: 8080},
							},
						},
					},
				},
			},
			expectError: true,
			errorMsg:    "loadbalancers.nginx.name is required",
		},
		{
			name: "Empty Envoy Name",
			config: &config.Config{
				LoadBalancers: config.LoadBalancersConfig{
					Envoy: []config.EnvoyConfig{
						{
							Name: "",
							Addresses: []config.AddressConfig{
								{Protocol: "http", IP: "10.0.0.1", Port: 8080},
							},
						},
					},
				},
			},
			expectError: true,
			errorMsg:    "loadbalancers.envoy.name is required",
		},
		{
			name: "Duplicate LoadBalancer Names",
			config: &config.Config{
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
			name: "Invalid IP in Nginx Address",
			config: &config.Config{
				LoadBalancers: config.LoadBalancersConfig{
					Nginx: []config.NginxConfig{
						{
							Name:      "nginx1",
							ListAPI:   "/list",
							AddAPI:    "/add",
							RemoveAPI: "/remove",
							Addresses: []config.AddressConfig{
								{Protocol: "http", IP: "invalid-ip", Port: 8080},
							},
						},
					},
				},
			},
			expectError: true,
			errorMsg:    "loadbalancers.nginx 'nginx1': invalid IP in addresses: invalid-ip",
		},
		{
			name: "Invalid Port in Envoy Address",
			config: &config.Config{
				LoadBalancers: config.LoadBalancersConfig{
					Envoy: []config.EnvoyConfig{
						{
							Name: "envoy1",
							Addresses: []config.AddressConfig{
								{Protocol: "http", IP: "10.0.0.1", Port: 70000},
							},
						},
					},
				},
			},
			expectError: true,
			errorMsg:    "loadbalancers.envoy 'envoy1': port must be between 0 and 65535: 70000",
		},
		{
			name: "Invalid Protocol in Nginx Address",
			config: &config.Config{
				LoadBalancers: config.LoadBalancersConfig{
					Nginx: []config.NginxConfig{
						{
							Name:      "nginx1",
							ListAPI:   "/list",
							AddAPI:    "/add",
							RemoveAPI: "/remove",
							Addresses: []config.AddressConfig{
								{Protocol: "ftp", IP: "192.168.1.1", Port: 8080},
							},
						},
					},
				},
			},
			expectError: true,
			errorMsg:    "loadbalancers.nginx 'nginx1': protocol must be one of: http, https, grpc; got: ftp",
		},
		{
			name: "Missing Hostname for HTTPS in Envoy",
			config: &config.Config{
				LoadBalancers: config.LoadBalancersConfig{
					Envoy: []config.EnvoyConfig{
						{
							Name: "envoy1",
							Addresses: []config.AddressConfig{
								{Protocol: "https", IP: "10.0.0.1", Port: 443},
							},
						},
					},
				},
			},
			expectError: true,
			errorMsg:    "loadbalancers.envoy 'envoy1': hostname is required for https protocol",
		},
		{
			name: "Invalid Hostname in Envoy",
			config: &config.Config{
				LoadBalancers: config.LoadBalancersConfig{
					Envoy: []config.EnvoyConfig{
						{
							Name: "envoy1",
							Addresses: []config.AddressConfig{
								{Protocol: "https", IP: "10.0.0.1", Port: 443, Hostname: "invalid@domain"},
							},
						},
					},
				},
			},
			expectError: true,
			errorMsg:    "loadbalancers.envoy 'envoy1': invalid hostname: invalid@domain",
		},
		{
			name:        "Empty LoadBalancers",
			config:      &config.Config{LoadBalancers: config.LoadBalancersConfig{Nginx: []config.NginxConfig{}, Envoy: []config.EnvoyConfig{}}},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := &LoadBalancersValidator{}
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
