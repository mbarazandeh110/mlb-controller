package validator

import (
	"testing"
	"time"

	"mlb-controller/internal/domain/config"

	"github.com/stretchr/testify/assert"
)

func TestNginxValidator_Validate(t *testing.T) {
	tests := []struct {
		name        string
		config      *config.Config
		expectError bool
		errorMsg    string
	}{
		{
			name: "Valid Nginx Config",
			config: &config.Config{
				LoadBalancers: config.LoadBalancersConfig{
					Nginx: []config.NginxConfig{
						{
							Name:               "nginx1",
							ListAPI:            "/list",
							AddAPI:             "/add",
							RemoveAPI:          "/remove",
							UpstreamSyncPeriod: 5 * time.Second,
							FailTimeout:        60 * time.Second,
							RequestTimeout:     30 * time.Second,
						},
					},
				},
			},
			expectError: false,
		},
		{
			name: "Empty ListAPI",
			config: &config.Config{
				LoadBalancers: config.LoadBalancersConfig{
					Nginx: []config.NginxConfig{
						{
							Name:               "nginx1",
							ListAPI:            "",
							AddAPI:             "/add",
							RemoveAPI:          "/remove",
							UpstreamSyncPeriod: 5 * time.Second,
							FailTimeout:        60 * time.Second,
							RequestTimeout:     30 * time.Second,
						},
					},
				},
			},
			expectError: true,
			errorMsg:    "nginx 'nginx1': list_api is required",
		},
		{
			name: "Empty AddAPI",
			config: &config.Config{
				LoadBalancers: config.LoadBalancersConfig{
					Nginx: []config.NginxConfig{
						{
							Name:               "nginx1",
							ListAPI:            "/list",
							AddAPI:             "",
							RemoveAPI:          "/remove",
							UpstreamSyncPeriod: 5 * time.Second,
							FailTimeout:        60 * time.Second,
							RequestTimeout:     30 * time.Second,
						},
					},
				},
			},
			expectError: true,
			errorMsg:    "nginx 'nginx1': add_api is required",
		},
		{
			name: "Empty RemoveAPI",
			config: &config.Config{
				LoadBalancers: config.LoadBalancersConfig{
					Nginx: []config.NginxConfig{
						{
							Name:               "nginx1",
							ListAPI:            "/list",
							AddAPI:             "/add",
							RemoveAPI:          "",
							UpstreamSyncPeriod: 5 * time.Second,
							FailTimeout:        60 * time.Second,
							RequestTimeout:     30 * time.Second,
						},
					},
				},
			},
			expectError: true,
			errorMsg:    "nginx 'nginx1': remove_api is required",
		},
		{
			name: "Negative UpstreamSyncPeriod",
			config: &config.Config{
				LoadBalancers: config.LoadBalancersConfig{
					Nginx: []config.NginxConfig{
						{
							Name:               "nginx1",
							ListAPI:            "/list",
							AddAPI:             "/add",
							RemoveAPI:          "/remove",
							UpstreamSyncPeriod: -5 * time.Second,
							FailTimeout:        60 * time.Second,
							RequestTimeout:     30 * time.Second,
						},
					},
				},
			},
			expectError: true,
			errorMsg:    "nginx 'nginx1': upstream_sync_period must be non-negative",
		},
		{
			name: "Negative FailTimeout",
			config: &config.Config{
				LoadBalancers: config.LoadBalancersConfig{
					Nginx: []config.NginxConfig{
						{
							Name:               "nginx1",
							ListAPI:            "/list",
							AddAPI:             "/add",
							RemoveAPI:          "/remove",
							UpstreamSyncPeriod: 5 * time.Second,
							FailTimeout:        -60 * time.Second,
							RequestTimeout:     30 * time.Second,
						},
					},
				},
			},
			expectError: true,
			errorMsg:    "nginx 'nginx1': fail_timeout must be non-negative",
		},
		{
			name: "Negative RequestTimeout",
			config: &config.Config{
				LoadBalancers: config.LoadBalancersConfig{
					Nginx: []config.NginxConfig{
						{
							Name:               "nginx1",
							ListAPI:            "/list",
							AddAPI:             "/add",
							RemoveAPI:          "/remove",
							UpstreamSyncPeriod: 5 * time.Second,
							FailTimeout:        60 * time.Second,
							RequestTimeout:     -30 * time.Second,
						},
					},
				},
			},
			expectError: true,
			errorMsg:    "nginx 'nginx1': request_timeout must be non-negative",
		},
		{
			name: "Empty Nginx Config List",
			config: &config.Config{
				LoadBalancers: config.LoadBalancersConfig{
					Nginx: []config.NginxConfig{},
				},
			},
			expectError: false,
		},
		{
			name: "Multiple Valid Nginx Configs",
			config: &config.Config{
				LoadBalancers: config.LoadBalancersConfig{
					Nginx: []config.NginxConfig{
						{
							Name:               "nginx1",
							ListAPI:            "/list",
							AddAPI:             "/add",
							RemoveAPI:          "/remove",
							UpstreamSyncPeriod: 5 * time.Second,
							FailTimeout:        60 * time.Second,
							RequestTimeout:     30 * time.Second,
						},
						{
							Name:               "nginx2",
							ListAPI:            "/list2",
							AddAPI:             "/add2",
							RemoveAPI:          "/remove2",
							UpstreamSyncPeriod: 3 * time.Second,
							FailTimeout:        30 * time.Second,
							RequestTimeout:     20 * time.Second,
						},
					},
				},
			},
			expectError: false,
		},
		{
			name: "Zero Timeouts",
			config: &config.Config{
				LoadBalancers: config.LoadBalancersConfig{
					Nginx: []config.NginxConfig{
						{
							Name:               "nginx1",
							ListAPI:            "/list",
							AddAPI:             "/add",
							RemoveAPI:          "/remove",
							UpstreamSyncPeriod: 0,
							FailTimeout:        0,
							RequestTimeout:     0,
						},
					},
				},
			},
			expectError: false,
		},
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
