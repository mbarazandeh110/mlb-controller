package validator

import (
	"testing"

	"mlb-controller/internal/domain/config"

	"github.com/stretchr/testify/assert"
)

func TestGlobalIPReplacementValidator_Validate(t *testing.T) {
	tests := []struct {
		name        string
		config      *config.Config
		expectError bool
		errorMsg    string
	}{
		{
			name: "Valid Config",
			config: &config.Config{
				GlobalIPReplacementList: config.GlobalIPReplacementList{
					Net: []config.GlobalNetReplacement{
						{
							Name: "net1",
							Nets: []config.NetConfig{
								{Source: "192.168.1.0", Target: "10.0.0.0", Mask: 24},
							},
						},
					},
					IP: []config.GlobalIPReplacement{
						{
							Name: "ip1",
							IPs:  []config.IPConfig{{Source: "192.168.1.1", Target: "10.0.0.1"}},
						},
					},
				},
			},
			expectError: false,
		},
		{
			name: "Empty Net Name",
			config: &config.Config{
				GlobalIPReplacementList: config.GlobalIPReplacementList{
					Net: []config.GlobalNetReplacement{
						{
							Name: "",
							Nets: []config.NetConfig{
								{Source: "192.168.1.0", Target: "10.0.0.0", Mask: 24},
							},
						},
					},
				},
			},
			expectError: true,
			errorMsg:    "global_ip_replacement_list.net.name is required",
		},
		{
			name: "Duplicate Net Name",
			config: &config.Config{
				GlobalIPReplacementList: config.GlobalIPReplacementList{
					Net: []config.GlobalNetReplacement{
						{
							Name: "net1",
							Nets: []config.NetConfig{
								{Source: "192.168.1.0", Target: "10.0.0.0", Mask: 24},
							},
						},
						{
							Name: "net1",
							Nets: []config.NetConfig{
								{Source: "192.168.2.0", Target: "10.0.1.0", Mask: 24},
							},
						},
					},
				},
			},
			expectError: true,
			errorMsg:    "global_ip_replacement_list.net.name 'net1' must be unique",
		},
		{
			name: "Invalid IP in Net",
			config: &config.Config{
				GlobalIPReplacementList: config.GlobalIPReplacementList{
					Net: []config.GlobalNetReplacement{
						{
							Name: "net1",
							Nets: []config.NetConfig{
								{Source: "invalid-ip", Target: "10.0.0.0", Mask: 24},
							},
						},
					},
				},
			},
			expectError: true,
			errorMsg:    "invalid IP in global_ip_replacement_list.net: source=invalid-ip, target=10.0.0.0",
		},
		{
			name: "Invalid Mask",
			config: &config.Config{
				GlobalIPReplacementList: config.GlobalIPReplacementList{
					Net: []config.GlobalNetReplacement{
						{
							Name: "net1",
							Nets: []config.NetConfig{
								{Source: "192.168.1.0", Target: "10.0.0.0", Mask: 33},
							},
						},
					},
				},
			},
			expectError: true,
			errorMsg:    "global_ip_replacement_list.net.mask must be between 0 and 32",
		},
		{
			name: "Overlapping Nets-1",
			config: &config.Config{
				GlobalIPReplacementList: config.GlobalIPReplacementList{
					Net: []config.GlobalNetReplacement{
						{
							Name: "net1",
							Nets: []config.NetConfig{
								{Source: "192.168.1.0", Target: "10.0.0.0", Mask: 24},
								{Source: "192.168.1.0", Target: "10.0.1.0", Mask: 24},
							},
						},
					},
				},
			},
			expectError: true,
			errorMsg:    "global_ip_replacement_list.net source '192.168.1.0/24' and '192.168.1.0/24' have overlap",
		},
		{
			name: "Overlapping Nets-2",
			config: &config.Config{
				GlobalIPReplacementList: config.GlobalIPReplacementList{
					Net: []config.GlobalNetReplacement{
						{
							Name: "net1",
							Nets: []config.NetConfig{
								{Source: "192.168.3.0", Target: "10.0.0.0", Mask: 24},
								{Source: "192.168.2.0", Target: "10.0.1.0", Mask: 23},
							},
						},
					},
				},
			},
			expectError: true,
			errorMsg:    "global_ip_replacement_list.net source '192.168.3.0/24' and '192.168.2.0/23' have overlap",
		},
		{
			name: "Overlapping Nets-3",
			config: &config.Config{
				GlobalIPReplacementList: config.GlobalIPReplacementList{
					Net: []config.GlobalNetReplacement{
						{
							Name: "net1",
							Nets: []config.NetConfig{
								{Source: "192.168.2.0", Target: "10.0.1.0", Mask: 23},
								{Source: "192.168.3.0", Target: "10.0.0.0", Mask: 24},
							},
						},
					},
				},
			},
			expectError: true,
			errorMsg:    "global_ip_replacement_list.net source '192.168.2.0/23' and '192.168.3.0/24' have overlap",
		},
		{
			name: "Overlapping Nets-4",
			config: &config.Config{
				GlobalIPReplacementList: config.GlobalIPReplacementList{
					Net: []config.GlobalNetReplacement{
						{
							Name: "net1",
							Nets: []config.NetConfig{
								{Source: "192.168.1.0", Target: "10.0.1.0", Mask: 25},
								{Source: "192.168.1.0", Target: "10.0.0.0", Mask: 24},
							},
						},
					},
				},
			},
			expectError: true,
			errorMsg:    "global_ip_replacement_list.net source '192.168.1.0/25' and '192.168.1.0/24' have overlap",
		},
		{
			name: "Empty IP Name",
			config: &config.Config{
				GlobalIPReplacementList: config.GlobalIPReplacementList{
					IP: []config.GlobalIPReplacement{
						{
							Name: "",
							IPs:  []config.IPConfig{{Source: "192.168.1.1", Target: "10.0.0.1"}},
						},
					},
				},
			},
			expectError: true,
			errorMsg:    "global_ip_replacement_list.ip.name is required",
		},
		{
			name: "Duplicate IP Name",
			config: &config.Config{
				GlobalIPReplacementList: config.GlobalIPReplacementList{
					IP: []config.GlobalIPReplacement{
						{
							Name: "ip1",
							IPs:  []config.IPConfig{{Source: "192.168.1.1", Target: "10.0.0.1"}},
						},
						{
							Name: "ip1",
							IPs:  []config.IPConfig{{Source: "192.168.1.2", Target: "10.0.0.2"}},
						},
					},
				},
			},
			expectError: true,
			errorMsg:    "global_ip_replacement_list.ip.name 'ip1' must be unique",
		},
		{
			name: "Invalid IP in IPs",
			config: &config.Config{
				GlobalIPReplacementList: config.GlobalIPReplacementList{
					IP: []config.GlobalIPReplacement{
						{
							Name: "ip1",
							IPs:  []config.IPConfig{{Source: "invalid-ip", Target: "10.0.0.1"}},
						},
					},
				},
			},
			expectError: true,
			errorMsg:    "invalid IP in global_ip_replacement_list.ip: source=invalid-ip, target=10.0.0.1",
		},
		{
			name: "Duplicate Source IP",
			config: &config.Config{
				GlobalIPReplacementList: config.GlobalIPReplacementList{
					IP: []config.GlobalIPReplacement{
						{
							Name: "ip1",
							IPs: []config.IPConfig{
								{Source: "192.168.1.1", Target: "10.0.0.1"},
								{Source: "192.168.1.1", Target: "10.0.0.2"},
							},
						},
					},
				},
			},
			expectError: true,
			errorMsg:    "global_ip_replacement_list.ip source '192.168.1.1' must be unique",
		},
		{
			name: "Empty Lists",
			config: &config.Config{
				GlobalIPReplacementList: config.GlobalIPReplacementList{
					Net: []config.GlobalNetReplacement{},
					IP:  []config.GlobalIPReplacement{},
				},
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := &GlobalIPReplacementValidator{}
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
