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
						Protocol:           "http",
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
					config.NginxConfig{Name: "nginx2", Protocol: "http"},
				},
			},
		}, true, "list_api is required"},
		{"Negative FailTimeout", &config.Config{
			LoadBalancers: config.LoadBalancersConfig{
				LoadBalancers: []config.LoadBalancerConfig{
					config.NginxConfig{
						Name:        "nginx3",
						ListAPI:     "/list",
						AddAPI:      "/add",
						RemoveAPI:   "/remove",
						Protocol:    "http",
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
						Name:      "nginx4",
						Protocol:  "ftp",
						Addresses: []config.AddressConfig{{IP: "192.168.1.1", Port: 80}},
					},
				},
			},
		}, true, "protocol must be one of: http, https, got: ftp"},
		{"valid Protocol (http)", &config.Config{
			LoadBalancers: config.LoadBalancersConfig{
				LoadBalancers: []config.LoadBalancerConfig{
					config.NginxConfig{
						Type:      "nginx",
						Name:      "nginx5",
						ListAPI:   "/list",
						AddAPI:    "/add",
						RemoveAPI: "/remove",
						Protocol:  "http",
						Addresses: []config.AddressConfig{{IP: "192.168.1.1", Port: 80}},
					},
				},
			},
		}, false, ""},
		{"valid Protocol (https)", &config.Config{
			LoadBalancers: config.LoadBalancersConfig{
				LoadBalancers: []config.LoadBalancerConfig{
					config.NginxConfig{
						Type:      "nginx",
						Name:      "nginx6",
						ListAPI:   "/list",
						AddAPI:    "/add",
						RemoveAPI: "/remove",
						Protocol:  "https",
						Hostname:  "test.com",
						Addresses: []config.AddressConfig{{IP: "192.168.1.1", Port: 80}},
					},
				},
			},
		}, false, ""},
		{"Hostname is required", &config.Config{
			LoadBalancers: config.LoadBalancersConfig{
				LoadBalancers: []config.LoadBalancerConfig{
					config.NginxConfig{
						Type:      "nginx",
						Name:      "nginx7",
						ListAPI:   "/list",
						AddAPI:    "/add",
						RemoveAPI: "/remove",
						Protocol:  "https",
						Addresses: []config.AddressConfig{{IP: "192.168.1.1", Port: 80}},
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
