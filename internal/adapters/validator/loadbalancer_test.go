package validator

import (
	"testing"

	"mlb-controller/internal/domain/config"

	"github.com/stretchr/testify/assert"
)

func TestLoadBalancerValidator_Validate(t *testing.T) {
	tests := []struct {
		name        string
		config      *config.Config
		expectError bool
		errorMsg    string
	}{
		{
			name: "Valid Config",
			config: &config.Config{
				LoadBalancers: config.LoadBalancersConfig{
					Nginx: []config.NginxConfig{
						{
							Name: "nginx1",
							Addresses: []config.AddressConfig{
								{Protocol: "http", IP: "192.168.1.1", Port: 8080},
							},
							ListAPI:   "/list",
							AddAPI:    "/add",
							RemoveAPI: "/remove",
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
							Name: "",
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
			name: "Duplicate Names",
			config: &config.Config{
				LoadBalancers: config.LoadBalancersConfig{
					Nginx: []config.NginxConfig{
						{
							Name: "lb1",
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
			name: "Invalid IP in Address",
			config: &config.Config{
				LoadBalancers: config.LoadBalancersConfig{
					Nginx: []config.NginxConfig{
						{
							Name: "nginx1",
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
			name: "Invalid Port",
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
			name: "Invalid Protocol",
			config: &config.Config{
				LoadBalancers: config.LoadBalancersConfig{
					Nginx: []config.NginxConfig{
						{
							Name: "nginx1",
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
			name: "Missing Hostname for HTTPS",
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
			name: "Invalid Hostname",
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
			name: "Valid IP Replacement",
			config: &config.Config{
				GlobalIPReplacementList: config.GlobalIPReplacementList{
					Net: []config.GlobalNetReplacement{
						{Name: "global-net", Nets: []config.NetConfig{{Source: "192.168.2.0", Target: "10.0.2.0", Mask: 24}}},
					},
					IP: []config.GlobalIPReplacement{
						{Name: "global-ip", IPs: []config.IPConfig{{Source: "192.168.1.2", Target: "10.0.0.2"}}},
					},
				},
				LoadBalancers: config.LoadBalancersConfig{
					Nginx: []config.NginxConfig{
						{
							Name:          "nginx1",
							IPReplacement: true,
							IPReplacementList: config.IPReplacementList{
								Nets:       []config.NetConfig{{Source: "192.168.1.0", Target: "10.0.0.0", Mask: 24}},
								IPs:        []config.IPConfig{{Source: "192.168.1.1", Target: "10.0.0.1"}},
								GlobalNets: []string{"global-net"},
								GlobalIPs:  []string{"global-ip"},
							},
							Addresses: []config.AddressConfig{
								{Protocol: "http", IP: "192.168.1.1", Port: 8080},
							},
						},
					},
				},
			},
			expectError: false,
		},
		{
			name: "Invalid Global Net Reference",
			config: &config.Config{
				LoadBalancers: config.LoadBalancersConfig{
					Nginx: []config.NginxConfig{
						{
							Name:          "nginx1",
							IPReplacement: true,
							IPReplacementList: config.IPReplacementList{
								GlobalNets: []string{"nonexistent-net"},
							},
						},
					},
				},
			},
			expectError: true,
			errorMsg:    "loadbalancers.nginx 'nginx1': global_nets 'nonexistent-net' does not exist in global_ip_replacement_list.net",
		},
		{
			name: "Network Overlap with Global",
			config: &config.Config{
				GlobalIPReplacementList: config.GlobalIPReplacementList{
					Net: []config.GlobalNetReplacement{
						{Name: "global-net", Nets: []config.NetConfig{{Source: "192.168.1.0", Target: "10.0.0.0", Mask: 24}}},
					},
				},
				LoadBalancers: config.LoadBalancersConfig{
					Nginx: []config.NginxConfig{
						{
							Name:          "nginx1",
							IPReplacement: true,
							IPReplacementList: config.IPReplacementList{
								Nets:       []config.NetConfig{{Source: "192.168.1.0", Target: "10.0.1.0", Mask: 24}},
								GlobalNets: []string{"global-net"},
							},
						},
					},
				},
			},
			expectError: true,
			errorMsg:    "network overlap detected: global_nets '192.168.1.0/24' overlaps with nets '192.168.1.0/24' in load balancer",
		},
		{
			name: "IP Overlap with Global",
			config: &config.Config{
				GlobalIPReplacementList: config.GlobalIPReplacementList{
					IP: []config.GlobalIPReplacement{
						{Name: "global-ip", IPs: []config.IPConfig{{Source: "192.168.1.1", Target: "10.0.0.1"}}},
					},
				},
				LoadBalancers: config.LoadBalancersConfig{
					Nginx: []config.NginxConfig{
						{
							Name:          "nginx1",
							IPReplacement: true,
							IPReplacementList: config.IPReplacementList{
								IPs:       []config.IPConfig{{Source: "192.168.1.1", Target: "10.0.0.2"}},
								GlobalIPs: []string{"global-ip"},
							},
						},
					},
				},
			},
			expectError: true,
			errorMsg:    "IP overlap detected: global_ips '192.168.1.1' overlaps with ips '192.168.1.1' in load balancer",
		},
		{
			name: "Empty LoadBalancers",
			config: &config.Config{
				LoadBalancers: config.LoadBalancersConfig{
					Nginx: []config.NginxConfig{},
					Envoy: []config.EnvoyConfig{},
				},
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := &LoadBalancerValidator{}
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
