package loadbalancer

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"mlb-controller/internal/adapters/logging"
	"mlb-controller/internal/domain/config"
)

type dummyLBConfig struct {
	config.NginxConfig
	customName string
	customType string
}

func (d dummyLBConfig) GetName() string { return d.customName }
func (d dummyLBConfig) GetType() string { return d.customType }

func TestLoadBalancerFactory(t *testing.T) {
	logger := logging.NewNop()
	factory := NewLoadBalancerFactory()

	t.Run("Create NginxAdapter with value struct", func(t *testing.T) {
		nginxCfg := config.NginxConfig{
			Name:            "nginx-1",
			Type:            "nginx",
			Protocol:        "http",
			Addresses:       []config.AddressConfig{{IP: "127.0.0.1", Port: 8080}},
			ListAPI:         "/list",
			AddAPI:          "/add",
			RemoveAPI:       "/remove",
			RequestPoolSize: 2,
			RequestTimeout:  time.Second,
		}
		adapter, err := factory.CreateLoadBalancerAdapter(nginxCfg, logger)
		require.NoError(t, err)
		assert.NotNil(t, adapter)
	})

	t.Run("Create NginxAdapter with pointer struct", func(t *testing.T) {
		nginxCfg := &config.NginxConfig{
			Name:            "nginx-ptr",
			Type:            "nginx",
			Protocol:        "http",
			Addresses:       []config.AddressConfig{{IP: "127.0.0.1", Port: 8080}},
			ListAPI:         "/list",
			AddAPI:          "/add",
			RemoveAPI:       "/remove",
			RequestPoolSize: 2,
			RequestTimeout:  time.Second,
		}
		adapter, err := factory.CreateLoadBalancerAdapter(nginxCfg, logger)
		require.NoError(t, err)
		assert.NotNil(t, adapter)
	})

	t.Run("Create NginxAdapter fails when concrete type mismatch", func(t *testing.T) {
		dummy := dummyLBConfig{customName: "fake-nginx", customType: "nginx"}
		adapter, err := factory.CreateLoadBalancerAdapter(dummy, logger)
		assert.Error(t, err)
		assert.Nil(t, adapter)
		assert.Contains(t, err.Error(), "failed to cast load balancer config to NginxConfig")
	})

	t.Run("Envoy adapter placeholder returns error", func(t *testing.T) {
		envoyCfg := config.EnvoyConfig{
			Name: "envoy-1",
			Type: "envoy",
		}
		adapter, err := factory.CreateLoadBalancerAdapter(envoyCfg, logger)
		assert.Error(t, err)
		assert.Nil(t, adapter)
		assert.Contains(t, err.Error(), "envoy adapter not implemented")
	})

	t.Run("Unsupported load balancer type returns error", func(t *testing.T) {
		dummy := dummyLBConfig{customName: "haproxy-1", customType: "haproxy"}
		adapter, err := factory.CreateLoadBalancerAdapter(dummy, logger)
		assert.Error(t, err)
		assert.Nil(t, adapter)
		assert.Contains(t, err.Error(), "unsupported load balancer type: haproxy")
	})
}
