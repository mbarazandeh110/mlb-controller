package util

import (
	"mlb-controller/internal/domain/config"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsValidIP(t *testing.T) {
	tests := []struct {
		name string
		ip   string
		want bool
	}{
		{"Valid IPv4", "192.168.1.1", true},
		{"Valid IPv6", "2001:db8::1", true},
		{"Invalid IP", "invalid", false},
		{"Empty", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, IsValidIP(tt.ip))
		})
	}
}

func TestIsValidDomain(t *testing.T) {
	tests := []struct {
		name     string
		hostname string
		want     bool
	}{
		{"Valid Domain", "example.com", true},
		{"Valid Subdomain", "sub.example.com", true},
		{"Invalid Domain", "example..com", false},
		{"Empty", "", false},
		{"Invalid Chars", "example@com", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, IsValidDomain(tt.hostname))
		})
	}
}

func TestIsValidPort(t *testing.T) {
	tests := []struct {
		name string
		port int
		want bool
	}{
		{"Valid Port", 8080, true},
		{"Min Port", 0, true},
		{"Max Port", 65535, true},
		{"Negative", -1, false},
		{"Too High", 65536, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, IsValidPort(tt.port))
		})
	}
}

func TestIsNetworkOverlap(t *testing.T) {
	tests := []struct {
		name  string
		ip1   string
		mask1 int
		ip2   string
		mask2 int
		want  bool
	}{
		{"Overlap Same Network", "192.168.1.0", 24, "192.168.1.10", 24, true},
		{"No Overlap", "192.168.1.0", 24, "10.0.0.0", 24, false},
		{"Invalid IP", "invalid", 24, "10.0.0.0", 24, false},
		{"Subset Overlap", "192.168.0.0", 16, "192.168.1.0", 24, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, IsNetworkOverlap(tt.ip1, tt.mask1, tt.ip2, tt.mask2))
		})
	}
}

func TestValidateIPReplacementList(t *testing.T) {
	tests := []struct {
		name       string
		list       config.IPReplacementList
		globalList config.GlobalIPReplacementList
		wantErr    bool
	}{
		{"Valid List", config.IPReplacementList{
			Nets: []config.NetConfig{{Source: "192.168.1.0", Target: "10.0.0.0", Mask: 24}},
			IPs:  []config.IPConfig{{Source: "192.168.1.1", Target: "10.0.0.1"}},
		}, config.GlobalIPReplacementList{
			Net: []config.GlobalNetReplacement{{Name: "net1"}},
			IP:  []config.GlobalIPReplacement{{Name: "ip1"}},
		}, false},
		{"Invalid IP", config.IPReplacementList{
			Nets: []config.NetConfig{{Source: "invalid", Target: "10.0.0.0", Mask: 24}},
		}, config.GlobalIPReplacementList{}, true},
		{"Invalid Mask", config.IPReplacementList{
			Nets: []config.NetConfig{{Source: "192.168.1.0", Target: "10.0.0.0", Mask: 33}},
		}, config.GlobalIPReplacementList{}, true},
		{"Overlap Nets", config.IPReplacementList{
			Nets: []config.NetConfig{
				{Source: "192.168.1.0", Target: "10.0.0.0", Mask: 24},
				{Source: "192.168.1.0", Target: "10.0.1.0", Mask: 24},
			},
		}, config.GlobalIPReplacementList{}, true},
		{"Invalid Global Ref", config.IPReplacementList{
			GlobalNets: []string{"nonexistent"},
		}, config.GlobalIPReplacementList{}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateIPReplacementList(tt.list, tt.globalList)
			assert.Equal(t, tt.wantErr, err != nil)
		})
	}
}
