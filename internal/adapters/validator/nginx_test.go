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
