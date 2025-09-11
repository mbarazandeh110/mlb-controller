package config

import (
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNewViperLoader(t *testing.T) {
	loader := NewViperLoader("test.yaml")
	assert.Equal(t, "test.yaml", loader.path)
	assert.NotNil(t, loader.validator)
}

func TestStringToDurationHookFunc(t *testing.T) {
	hook := StringToDurationHookFunc()
	val, err := hook(reflect.TypeOf(""), reflect.TypeOf(time.Duration(0)), "10s")
	assert.NoError(t, err)
	assert.Equal(t, 10*time.Second, val)

	val, err = hook(reflect.TypeOf(0), reflect.TypeOf(time.Duration(0)), 0) // Non-string
	assert.NoError(t, err)
	assert.Equal(t, 0, val)
}

func TestViperLoader_Load(t *testing.T) {
	// Create temp config file
	content := `
global_upstream_sync_period: 5s
leader_election:
  enabled: true
  lease_name: test
  lease_namespace: default
log:
  level: info
  format: json
metrics:
  enabled: true
  port: 9090
  uri: /metrics
kubernetes:
  resync_period: 30s
global_ip_replacement_list:
  net:
    - name: net1
      nets:
        - source: 192.168.1.0
          target: 10.0.0.0
          mask: 24
  ip:
    - name: ip1
      ips:
        - source: 192.168.1.1
          target: 10.0.0.1
loadbalancers:
  - type: nginx
    name: nginx1
    addresses:
      - protocol: http
        ip: 192.168.1.1
        port: 80
    list_api: /list
    add_api: /add
    remove_api: /remove
  - type: envoy
    name: envoy1
    addresses:
      - protocol: grpc
        ip: 10.0.0.1
        port: 50051
`
	tmpFile, err := os.CreateTemp("", "config*.yaml")
	assert.NoError(t, err)
	defer os.Remove(tmpFile.Name())
	_, err = tmpFile.Write([]byte(content))
	assert.NoError(t, err)
	tmpFile.Close()

	loader := NewViperLoader(tmpFile.Name())
	cfg, err := loader.Load()
	assert.NoError(t, err)
	assert.NotNil(t, cfg)
	assert.Equal(t, 5*time.Second, cfg.GlobalUpstreamSyncPeriod)
	assert.Len(t, cfg.LoadBalancers.LoadBalancers, 2)

	// Test invalid config
	loader = NewViperLoader("invalid.yaml")
	_, err = loader.Load()
	assert.Error(t, err)
}

func TestMapstructureDecode(t *testing.T) {
	input := map[string]interface{}{"timeout": "10s"}
	var output struct {
		Timeout time.Duration `mapstructure:"timeout"`
	}
	err := mapstructureDecode(input, &output)
	assert.NoError(t, err)
	assert.Equal(t, 10*time.Second, output.Timeout)
}
