package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestApplyDefaultValues(t *testing.T) {
	cfg := &Config{}
	ApplyDefaultValues(cfg)

	assert.Equal(t, 10*time.Second, cfg.GlobalUpstreamSyncPeriod)
	assert.Equal(t, 15*time.Second, cfg.LeaderElection.LeaseDuration)
	assert.Equal(t, "info", cfg.Log.Level)
	assert.Equal(t, "json", cfg.Log.Format)
	assert.Equal(t, 9090, cfg.Metrics.Port)
	assert.Equal(t, "/metrics", cfg.Metrics.URI)
	assert.Equal(t, 30*time.Second, cfg.Kubernetes.ResyncPeriod)

	// Test LB defaults
	cfg.LoadBalancers.LoadBalancers = []LoadBalancerConfig{
		NginxConfig{},
		EnvoyConfig{},
	}
	ApplyDefaultValues(cfg)
	nginx := cfg.LoadBalancers.LoadBalancers[0].(NginxConfig)
	assert.Equal(t, 10*time.Second, nginx.UpstreamSyncPeriod)
	assert.Equal(t, 60*time.Second, nginx.FailTimeout)
	envoy := cfg.LoadBalancers.LoadBalancers[1].(EnvoyConfig)
	assert.Equal(t, 10*time.Second, envoy.UpstreamSyncPeriod)
	assert.Equal(t, 30*time.Second, envoy.RequestTimeout)
}
