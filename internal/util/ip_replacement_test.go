package util

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"mlb-controller/internal/domain/config"
)

type mockLBConfig struct {
	config.NginxConfig
	replacementList config.IPReplacementList
}

func (m mockLBConfig) GetIPReplacementList() config.IPReplacementList {
	return m.replacementList
}

func TestApplyIPReplacement(t *testing.T) {
	globalList := config.GlobalIPReplacementList{
		IP: []config.GlobalIPReplacement{
			{
				Name: "global-ips",
				IPs: []config.IPConfig{
					{Source: "10.0.0.10", Target: "192.168.1.10"},
				},
			},
		},
		Net: []config.GlobalNetReplacement{
			{
				Name: "global-nets",
				Nets: []config.NetConfig{
					{Source: "10.10.0.0", Target: "172.16.0.0", Mask: 16},
				},
			},
		},
	}

	tests := []struct {
		name       string
		nodeIP     string
		lbConfig   config.LoadBalancerConfig
		expectedIP string
	}{
		{
			name:   "Individual IP replacement from LB config",
			nodeIP: "10.0.0.1",
			lbConfig: mockLBConfig{
				replacementList: config.IPReplacementList{
					IPs: []config.IPConfig{
						{Source: "10.0.0.1", Target: "192.168.1.1"},
					},
				},
			},
			expectedIP: "192.168.1.1",
		},
		{
			name:   "Global IP replacement",
			nodeIP: "10.0.0.10",
			lbConfig: mockLBConfig{
				replacementList: config.IPReplacementList{
					GlobalIPs: []string{"global-ips"},
				},
			},
			expectedIP: "192.168.1.10",
		},
		{
			name:   "Subnet IPv4 replacement from LB config",
			nodeIP: "10.1.2.34",
			lbConfig: mockLBConfig{
				replacementList: config.IPReplacementList{
					Nets: []config.NetConfig{
						{Source: "10.1.2.0", Target: "192.168.100.0", Mask: 24},
					},
				},
			},
			expectedIP: "192.168.100.34",
		},
		{
			name:   "Subnet IPv6 replacement from LB config",
			nodeIP: "2001:db8:1::1234",
			lbConfig: mockLBConfig{
				replacementList: config.IPReplacementList{
					Nets: []config.NetConfig{
						{Source: "2001:db8:1::", Target: "2001:db8:2::", Mask: 64},
					},
				},
			},
			expectedIP: "2001:db8:2::1234",
		},
		{
			name:   "Global Subnet replacement",
			nodeIP: "10.10.5.6",
			lbConfig: mockLBConfig{
				replacementList: config.IPReplacementList{
					GlobalNets: []string{"global-nets"},
				},
			},
			expectedIP: "172.16.5.6",
		},
		{
			name:   "No matching replacement leaves IP unchanged",
			nodeIP: "8.8.8.8",
			lbConfig: mockLBConfig{
				replacementList: config.IPReplacementList{
					IPs: []config.IPConfig{
						{Source: "10.0.0.1", Target: "192.168.1.1"},
					},
				},
			},
			expectedIP: "8.8.8.8",
		},
		{
			name:       "Invalid node IP leaves input unchanged",
			nodeIP:     "not-an-ip",
			lbConfig:   mockLBConfig{},
			expectedIP: "not-an-ip",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := ApplyIPReplacement(tt.nodeIP, tt.lbConfig, globalList)
			assert.Equal(t, tt.expectedIP, res)
		})
	}
}
