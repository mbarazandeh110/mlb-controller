package validator

import (
	"mlb-controller/internal/domain/config"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNginxValidator_Validate(t *testing.T) {
	tests := []struct {
		name        string
		config      *config.Config
		expectError bool
		errorMsg    string
	}{
		{"Valid Nginx", &config.Config{
			LoadBalancers: config.LoadBalancersConfig{
				LoadBalancers: []config.LoadBalancerConfig{
					config.NginxConfig{
						Name:               "nginx1",
						ListAPI:            "/list",
						AddAPI:             "/add",
						RemoveAPI:          "/remove",
						UpstreamSyncPeriod: 10 * time.Second,
						FailTimeout:        60 * time.Second,
						RequestTimeout:     30 * time.Second,
					},
				},
			},
		}, false, ""},
		{"Missing ListAPI", &config.Config{
			LoadBalancers: config.LoadBalancersConfig{
				LoadBalancers: []config.LoadBalancerConfig{
					config.NginxConfig{Name: "nginx1"},
				},
			},
		}, true, "list_api is required"},
		{"Negative FailTimeout", &config.Config{
			LoadBalancers: config.LoadBalancersConfig{
				LoadBalancers: []config.LoadBalancerConfig{
					config.NginxConfig{
						Name:        "nginx1",
						ListAPI:     "/list",
						AddAPI:      "/add",
						RemoveAPI:   "/remove",
						FailTimeout: -60 * time.Second,
					},
				},
			},
		}, true, "fail_timeout must be non-negative"},
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
		}, true, "protocol must be one of: http, https, got: ftp"},
		{"valid Protocol (http)", &config.Config{
			LoadBalancers: config.LoadBalancersConfig{
				LoadBalancers: []config.LoadBalancerConfig{
					config.NginxConfig{
						Type:      "nginx",
						Name:      "nginx1",
						ListAPI:   "/list",
						AddAPI:    "/add",
						RemoveAPI: "/remove",
						Addresses: []config.AddressConfig{{Protocol: "http", IP: "192.168.1.1", Port: 80}},
					},
				},
			},
		}, false, ""},
		{"valid Protocol (https)", &config.Config{
			LoadBalancers: config.LoadBalancersConfig{
				LoadBalancers: []config.LoadBalancerConfig{
					config.NginxConfig{
						Type:      "nginx",
						Name:      "nginx1",
						ListAPI:   "/list",
						AddAPI:    "/add",
						RemoveAPI: "/remove",
						Addresses: []config.AddressConfig{{Protocol: "https", IP: "192.168.1.1", Port: 80, Hostname: "test.com"}},
					},
				},
			},
		}, false, ""},
		{"Hostname is required", &config.Config{
			LoadBalancers: config.LoadBalancersConfig{
				LoadBalancers: []config.LoadBalancerConfig{
					config.NginxConfig{
						Type:      "nginx",
						Name:      "nginx1",
						ListAPI:   "/list",
						AddAPI:    "/add",
						RemoveAPI: "/remove",
						Addresses: []config.AddressConfig{{Protocol: "https", IP: "192.168.1.1", Port: 80}},
					},
				},
			},
		}, true, "hostname is required for https protocol"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := &NginxValidator{}
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
