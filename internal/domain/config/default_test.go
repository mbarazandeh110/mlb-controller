package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestApplyDefaultValues(t *testing.T) {
	tests := []struct {
		name     string
		input    Config
		expected Config
	}{
		{
			name:  "Empty Config",
			input: Config{},
			expected: Config{
				GlobalUpstreamSyncPeriod: 10 * time.Second,
				LeaderElection: LeaderElectionConfig{
					LeaseDuration: 15 * time.Second,
					RenewDeadline: 10 * time.Second,
					RetryPeriod:   2 * time.Second,
				},
				Log: LogConfig{
					Level:  "info",
					Format: "json",
				},
				Metrics: MetricsConfig{
					Port: 9090,
					URI:  "/metrics",
				},
				Kubernetes: KubernetesConfig{
					ResyncPeriod: 30 * time.Second,
				},
				LoadBalancers: LoadBalancersConfig{
					Nginx: []NginxConfig{},
					Envoy: []EnvoyConfig{},
				},
			},
		},
		{
			name: "Partial Config with Nginx and Envoy",
			input: Config{
				LoadBalancers: LoadBalancersConfig{
					Nginx: []NginxConfig{{Name: "nginx1"}},
					Envoy: []EnvoyConfig{{Name: "envoy1"}},
				},
			},
			expected: Config{
				GlobalUpstreamSyncPeriod: 10 * time.Second,
				LeaderElection: LeaderElectionConfig{
					LeaseDuration: 15 * time.Second,
					RenewDeadline: 10 * time.Second,
					RetryPeriod:   2 * time.Second,
				},
				Log: LogConfig{
					Level:  "info",
					Format: "json",
				},
				Metrics: MetricsConfig{
					Port: 9090,
					URI:  "/metrics",
				},
				Kubernetes: KubernetesConfig{
					ResyncPeriod: 30 * time.Second,
				},
				LoadBalancers: LoadBalancersConfig{
					Nginx: []NginxConfig{{
						Name:               "nginx1",
						UpstreamSyncPeriod: 10 * time.Second,
						FailTimeout:        60 * time.Second,
						RequestTimeout:     30 * time.Second,
					}},
					Envoy: []EnvoyConfig{{
						Name:               "envoy1",
						UpstreamSyncPeriod: 10 * time.Second,
						RequestTimeout:     30 * time.Second,
					}},
				},
			},
		},
		{
			name: "Config with Some Values Set",
			input: Config{
				GlobalUpstreamSyncPeriod: 5 * time.Second,
				Log: LogConfig{
					Level: "debug",
				},
				Metrics: MetricsConfig{
					Port: 8080,
				},
			},
			expected: Config{
				GlobalUpstreamSyncPeriod: 5 * time.Second,
				LeaderElection: LeaderElectionConfig{
					LeaseDuration: 15 * time.Second,
					RenewDeadline: 10 * time.Second,
					RetryPeriod:   2 * time.Second,
				},
				Log: LogConfig{
					Level:  "debug",
					Format: "json",
				},
				Metrics: MetricsConfig{
					Port: 8080,
					URI:  "/metrics",
				},
				Kubernetes: KubernetesConfig{
					ResyncPeriod: 30 * time.Second,
				},
				LoadBalancers: LoadBalancersConfig{
					Nginx: []NginxConfig{},
					Envoy: []EnvoyConfig{},
				},
			},
		},
		{
			name: "Config with Nil LoadBalancers",
			input: Config{
				LoadBalancers: LoadBalancersConfig{
					Nginx: nil,
					Envoy: nil,
				},
			},
			expected: Config{
				GlobalUpstreamSyncPeriod: 10 * time.Second,
				LeaderElection: LeaderElectionConfig{
					LeaseDuration: 15 * time.Second,
					RenewDeadline: 10 * time.Second,
					RetryPeriod:   2 * time.Second,
				},
				Log: LogConfig{
					Level:  "info",
					Format: "json",
				},
				Metrics: MetricsConfig{
					Port: 9090,
					URI:  "/metrics",
				},
				Kubernetes: KubernetesConfig{
					ResyncPeriod: 30 * time.Second,
				},
				LoadBalancers: LoadBalancersConfig{
					Nginx: []NginxConfig{},
					Envoy: []EnvoyConfig{},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := tt.input
			ApplyDefaultValues(&cfg)
			assert.Equal(t, tt.expected, cfg)
		})
	}
}
