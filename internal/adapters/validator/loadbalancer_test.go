package validator

import (
	"mlb-controller/internal/domain/config"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLoadBalancerValidator_Validate(t *testing.T) {
	tests := []struct {
		name        string
		config      *config.Config
		expectError bool
		errorMsg    string
	}{
		{"Valid LB", &config.Config{
			LoadBalancers: config.LoadBalancersConfig{
				LoadBalancers: []config.LoadBalancerConfig{
					config.NginxConfig{
						Type:      "nginx",
						Name:      "nginx1",
						Addresses: []config.AddressConfig{{Protocol: "http", IP: "192.168.1.1", Port: 80}},
					},
				},
			},
		}, false, ""},
		{"Missing Name", &config.Config{
			LoadBalancers: config.LoadBalancersConfig{
				LoadBalancers: []config.LoadBalancerConfig{
					config.NginxConfig{Type: "nginx"},
				},
			},
		}, true, "name is required"},
		{"Duplicate Name", &config.Config{
			LoadBalancers: config.LoadBalancersConfig{
				LoadBalancers: []config.LoadBalancerConfig{
					config.NginxConfig{Type: "nginx", Name: "dup"},
					config.EnvoyConfig{Type: "envoy", Name: "dup"},
				},
			},
		}, true, "must be unique"},
		{"Invalid Protocol", &config.Config{
			LoadBalancers: config.LoadBalancersConfig{
				LoadBalancers: []config.LoadBalancerConfig{
					config.NginxConfig{
						Type:      "nginx",
						Name:      "nginx1",
						Addresses: []config.AddressConfig{{Protocol: "ftp", IP: "192.168.1.1", Port: 80}},
					},
				},
			},
		}, true, "protocol must be one of"},
		{"Invalid IP Replacement", &config.Config{
			LoadBalancers: config.LoadBalancersConfig{
				LoadBalancers: []config.LoadBalancerConfig{
					config.NginxConfig{
						Type:          "nginx",
						Name:          "nginx1",
						IPReplacement: true,
						IPReplacementList: config.IPReplacementList{
							Nets: []config.NetConfig{{Source: "invalid", Target: "10.0.0.0", Mask: 24}},
						},
					},
				},
			},
		}, true, "invalid IP"},
		{"Network Overlap with Global-1", &config.Config{
			GlobalIPReplacementList: config.GlobalIPReplacementList{
				Net: []config.GlobalNetReplacement{
					{Name: "net1", Nets: []config.NetConfig{{Source: "192.168.1.0", Target: "10.0.0.0", Mask: 24}}},
				},
			},
			LoadBalancers: config.LoadBalancersConfig{
				LoadBalancers: []config.LoadBalancerConfig{
					config.NginxConfig{
						Type:          "nginx",
						Name:          "nginx1",
						IPReplacement: true,
						IPReplacementList: config.IPReplacementList{
							GlobalNets: []string{"net1"},
							Nets:       []config.NetConfig{{Source: "192.168.1.0", Target: "10.0.1.0", Mask: 24}},
						},
					},
				},
			},
		}, true, "network overlap detected"},
		{"Network Overlap with Global-2", &config.Config{
			GlobalIPReplacementList: config.GlobalIPReplacementList{
				Net: []config.GlobalNetReplacement{
					{Name: "net1", Nets: []config.NetConfig{{Source: "192.168.2.0", Target: "10.0.0.0", Mask: 23}}},
				},
			},
			LoadBalancers: config.LoadBalancersConfig{
				LoadBalancers: []config.LoadBalancerConfig{
					config.NginxConfig{
						Type:          "nginx",
						Name:          "nginx1",
						IPReplacement: true,
						IPReplacementList: config.IPReplacementList{
							GlobalNets: []string{"net1"},
							Nets:       []config.NetConfig{{Source: "192.168.3.0", Target: "10.0.1.0", Mask: 24}},
						},
					},
				},
			},
		}, true, "network overlap detected"},
		{"Network Overlap with Global-3", &config.Config{
			GlobalIPReplacementList: config.GlobalIPReplacementList{
				Net: []config.GlobalNetReplacement{
					{Name: "net1", Nets: []config.NetConfig{{Source: "192.168.3.0", Target: "10.0.0.0", Mask: 24}}},
				},
			},
			LoadBalancers: config.LoadBalancersConfig{
				LoadBalancers: []config.LoadBalancerConfig{
					config.NginxConfig{
						Type:          "nginx",
						Name:          "nginx1",
						IPReplacement: true,
						IPReplacementList: config.IPReplacementList{
							GlobalNets: []string{"net1"},
							Nets:       []config.NetConfig{{Source: "192.168.2.0", Target: "10.0.1.0", Mask: 23}},
						},
					},
				},
			},
		}, true, "network overlap detected"},
		{"Network Overlap with Global-4", &config.Config{
			LoadBalancers: config.LoadBalancersConfig{
				LoadBalancers: []config.LoadBalancerConfig{
					config.NginxConfig{
						Type:          "nginx",
						Name:          "nginx1",
						IPReplacement: true,
						IPReplacementList: config.IPReplacementList{
							Nets: []config.NetConfig{{Source: "192.168.2.0", Target: "10.0.1.0", Mask: 24},
								{Source: "192.168.2.0", Target: "10.0.1.0", Mask: 23}},
						},
					},
				},
			},
		}, true, "loadbalancers.nginx 'nginx1': nets.net source '192.168.2.0/24' and '192.168.2.0/23' must not overlap"},
		{"Network Overlap with Global-5", &config.Config{
			LoadBalancers: config.LoadBalancersConfig{
				LoadBalancers: []config.LoadBalancerConfig{
					config.NginxConfig{
						Type:          "nginx",
						Name:          "nginx1",
						IPReplacement: true,
						IPReplacementList: config.IPReplacementList{
							Nets: []config.NetConfig{{Source: "192.168.2.0", Target: "10.0.1.0", Mask: 23},
								{Source: "192.168.3.0", Target: "10.0.1.0", Mask: 24}},
						},
					},
				},
			},
		}, true, "loadbalancers.nginx 'nginx1': nets.net source '192.168.2.0/23' and '192.168.3.0/24' must not overlap"},
		{"Network Overlap with Global-6", &config.Config{
			LoadBalancers: config.LoadBalancersConfig{
				LoadBalancers: []config.LoadBalancerConfig{
					config.NginxConfig{
						Type:          "nginx",
						Name:          "nginx1",
						IPReplacement: true,
						IPReplacementList: config.IPReplacementList{
							Nets: []config.NetConfig{{Source: "192.168.3.0", Target: "10.0.1.0", Mask: 24},
								{Source: "192.168.2.0", Target: "10.0.1.0", Mask: 23}},
						},
					},
				},
			},
		}, true, "loadbalancers.nginx 'nginx1': nets.net source '192.168.3.0/24' and '192.168.2.0/23' must not overlap"},
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
