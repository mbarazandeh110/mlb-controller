package validator

import (
	"mlb-controller/internal/domain/config"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// mockLoadBalancerConfig is a mock implementation of LoadBalancerConfig for testing.
type mockLoadBalancerConfig struct {
	name              string
	addresses         []config.AddressConfig
	ipReplacement     bool
	ipReplacementList config.IPReplacementList
	lbType            string
	hostname          string
	certPath          string
	keyPath           string
	caPath            string
	requestPoolSize   int
	requestTimeOut    time.Duration
	defaultBackend    *config.DefaultBackendConfig
}

func (m *mockLoadBalancerConfig) GetName() string                      { return m.name }
func (m *mockLoadBalancerConfig) GetAddresses() []config.AddressConfig { return m.addresses }
func (m *mockLoadBalancerConfig) GetIPReplacement() bool               { return m.ipReplacement }
func (m *mockLoadBalancerConfig) GetIPReplacementList() config.IPReplacementList {
	return m.ipReplacementList
}
func (m *mockLoadBalancerConfig) GetType() string                      { return m.lbType }
func (m *mockLoadBalancerConfig) GetHostName() string                  { return m.hostname }
func (m *mockLoadBalancerConfig) GetProtocol() string                  { return "https" }
func (m *mockLoadBalancerConfig) GetCertPath() string                  { return m.certPath }
func (m *mockLoadBalancerConfig) GetKeyPath() string                   { return m.keyPath }
func (m *mockLoadBalancerConfig) GetCAPath() string                    { return m.caPath }
func (m *mockLoadBalancerConfig) GetRequestPoolSize() int              { return m.requestPoolSize }
func (m *mockLoadBalancerConfig) GetRequestTimeOut() time.Duration     { return m.requestTimeOut }
func (m *mockLoadBalancerConfig) GetUpstreamSyncPeriod() time.Duration { return 0 }
func (m *mockLoadBalancerConfig) GetPruneOrphans() bool                { return false }
func (m *mockLoadBalancerConfig) GetAllowEmptyUpstream() bool          { return false }
func (m *mockLoadBalancerConfig) GetDefaultBackend() *config.DefaultBackendConfig {
	return m.defaultBackend
}
func (m *mockLoadBalancerConfig) SetDefaults(time.Duration, int) config.LoadBalancerConfig {
	return m
}

func TestLoadBalancerValidator_Validate(t *testing.T) {
	validator := &LoadBalancerValidator{}

	t.Run("ValidConfig", func(t *testing.T) {
		cfg := &config.Config{
			LoadBalancers: config.LoadBalancersConfig{
				LoadBalancers: []config.LoadBalancerConfig{
					&mockLoadBalancerConfig{
						name:      "test-lb",
						lbType:    "nginx",
						addresses: []config.AddressConfig{{IP: "192.168.1.1", Port: 8080}},
					},
				},
			},
		}
		err := validator.Validate(cfg)
		assert.NoError(t, err)
	})

	t.Run("DuplicateName", func(t *testing.T) {
		cfg := &config.Config{
			LoadBalancers: config.LoadBalancersConfig{
				LoadBalancers: []config.LoadBalancerConfig{
					&mockLoadBalancerConfig{name: "test-lb", lbType: "nginx"},
					&mockLoadBalancerConfig{name: "test-lb", lbType: "envoy"},
				},
			},
		}
		err := validator.Validate(cfg)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "name 'test-lb' must be unique")
	})

	t.Run("EmptyName", func(t *testing.T) {
		cfg := &config.Config{
			LoadBalancers: config.LoadBalancersConfig{
				LoadBalancers: []config.LoadBalancerConfig{
					&mockLoadBalancerConfig{name: "", lbType: "nginx"},
				},
			},
		}
		err := validator.Validate(cfg)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "name is required")
	})

	t.Run("InvalidIP", func(t *testing.T) {
		cfg := &config.Config{
			LoadBalancers: config.LoadBalancersConfig{
				LoadBalancers: []config.LoadBalancerConfig{
					&mockLoadBalancerConfig{
						name:      "test-lb",
						lbType:    "nginx",
						addresses: []config.AddressConfig{{IP: "invalid", Port: 8080}},
					},
				},
			},
		}
		err := validator.Validate(cfg)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid IP")
	})

	t.Run("InvalidPort", func(t *testing.T) {
		cfg := &config.Config{
			LoadBalancers: config.LoadBalancersConfig{
				LoadBalancers: []config.LoadBalancerConfig{
					&mockLoadBalancerConfig{
						name:      "test-lb",
						lbType:    "nginx",
						addresses: []config.AddressConfig{{IP: "192.168.1.1", Port: 65536}},
					},
				},
			},
		}
		err := validator.Validate(cfg)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "port must be between 0 and 65535")
	})

	t.Run("EmptyIPReplacementList", func(t *testing.T) {
		cfg := &config.Config{
			LoadBalancers: config.LoadBalancersConfig{
				LoadBalancers: []config.LoadBalancerConfig{
					&mockLoadBalancerConfig{
						name:              "test-lb",
						lbType:            "nginx",
						ipReplacement:     true,
						ipReplacementList: config.IPReplacementList{},
					},
				},
			},
		}
		err := validator.Validate(cfg)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "ip_replacement_list is empty")
	})

	t.Run("ValidDefaultBackend", func(t *testing.T) {
		cfg := &config.Config{
			LoadBalancers: config.LoadBalancersConfig{
				LoadBalancers: []config.LoadBalancerConfig{
					&mockLoadBalancerConfig{
						name:      "test-lb",
						lbType:    "nginx",
						addresses: []config.AddressConfig{{IP: "192.168.1.1", Port: 8080}},
						defaultBackend: &config.DefaultBackendConfig{
							IP:   "10.0.0.99",
							Port: 8080,
						},
					},
				},
			},
		}
		err := validator.Validate(cfg)
		assert.NoError(t, err)
	})

	t.Run("InvalidDefaultBackendIP", func(t *testing.T) {
		cfg := &config.Config{
			LoadBalancers: config.LoadBalancersConfig{
				LoadBalancers: []config.LoadBalancerConfig{
					&mockLoadBalancerConfig{
						name:      "test-lb",
						lbType:    "nginx",
						addresses: []config.AddressConfig{{IP: "192.168.1.1", Port: 8080}},
						defaultBackend: &config.DefaultBackendConfig{
							IP:   "not-an-ip",
							Port: 8080,
						},
					},
				},
			},
		}
		err := validator.Validate(cfg)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid IP in default_backend")
	})

	t.Run("InvalidDefaultBackendPort", func(t *testing.T) {
		cfg := &config.Config{
			LoadBalancers: config.LoadBalancersConfig{
				LoadBalancers: []config.LoadBalancerConfig{
					&mockLoadBalancerConfig{
						name:      "test-lb",
						lbType:    "nginx",
						addresses: []config.AddressConfig{{IP: "192.168.1.1", Port: 8080}},
						defaultBackend: &config.DefaultBackendConfig{
							IP:   "10.0.0.99",
							Port: 70000,
						},
					},
				},
			},
		}
		err := validator.Validate(cfg)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid port in default_backend")
	})
}

func TestLoadBalancerValidator_validateIPReplacementList(t *testing.T) {
	validator := &LoadBalancerValidator{}

	t.Run("ValidIPReplacementList", func(t *testing.T) {
		list := config.IPReplacementList{
			Nets:       []config.NetConfig{{Source: "192.168.1.0", Target: "10.0.0.0", Mask: 24}},
			IPs:        []config.IPConfig{{Source: "192.168.1.1", Target: "10.0.0.1"}},
			GlobalNets: []string{"global-net"},
			GlobalIPs:  []string{"global-ip"},
		}
		globalList := config.GlobalIPReplacementList{
			Net: []config.GlobalNetReplacement{{Name: "global-net", Nets: []config.NetConfig{{Source: "10.0.1.0", Target: "172.16.0.0", Mask: 24}}}},
			IP:  []config.GlobalIPReplacement{{Name: "global-ip", IPs: []config.IPConfig{{Source: "10.0.1.1", Target: "172.16.0.1"}}}},
		}
		err := validator.validateIPReplacementList(list, globalList, "test-lb", "nginx")
		assert.NoError(t, err)
	})

	t.Run("NetworkOverlap", func(t *testing.T) {
		list := config.IPReplacementList{
			Nets:       []config.NetConfig{{Source: "192.168.1.0", Target: "10.0.0.0", Mask: 24}},
			GlobalNets: []string{"global-net"},
		}
		globalList := config.GlobalIPReplacementList{
			Net: []config.GlobalNetReplacement{{Name: "global-net", Nets: []config.NetConfig{{Source: "192.168.1.0", Target: "172.16.0.0", Mask: 24}}}},
		}
		err := validator.validateIPReplacementList(list, globalList, "test-lb", "nginx")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "network overlap detected")
	})

	t.Run("IPOverlap", func(t *testing.T) {
		list := config.IPReplacementList{
			IPs:       []config.IPConfig{{Source: "192.168.1.1", Target: "10.0.0.1"}},
			GlobalIPs: []string{"global-ip"},
		}
		globalList := config.GlobalIPReplacementList{
			IP: []config.GlobalIPReplacement{{Name: "global-ip", IPs: []config.IPConfig{{Source: "192.168.1.1", Target: "172.16.0.1"}}}},
		}
		err := validator.validateIPReplacementList(list, globalList, "test-lb", "nginx")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "IP overlap detected")
	})

	t.Run("MultipleGlobalNets_OverlapBetweenThem", func(t *testing.T) {
		list := config.IPReplacementList{
			GlobalNets: []string{"g1", "g2"},
		}
		globalList := config.GlobalIPReplacementList{
			Net: []config.GlobalNetReplacement{
				{Name: "g1", Nets: []config.NetConfig{{Source: "192.168.1.0", Target: "10.0.0.0", Mask: 24}}},
				{Name: "g2", Nets: []config.NetConfig{{Source: "192.168.1.128", Target: "10.0.1.0", Mask: 25}}},
			},
		}
		err := validator.validateIPReplacementList(list, globalList, "test-lb", "nginx")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "network overlap detected between referenced global nets")
	})

	t.Run("MultipleGlobalIPs_DuplicateSourceAcrossGroups", func(t *testing.T) {
		list := config.IPReplacementList{
			GlobalIPs: []string{"ip1", "ip2"},
		}
		globalList := config.GlobalIPReplacementList{
			IP: []config.GlobalIPReplacement{
				{Name: "ip1", IPs: []config.IPConfig{{Source: "10.0.0.1", Target: "172.16.0.1"}}},
				{Name: "ip2", IPs: []config.IPConfig{{Source: "10.0.0.1", Target: "172.16.0.2"}}},
			},
		}
		err := validator.validateIPReplacementList(list, globalList, "test-lb", "nginx")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "duplicate source IP '10.0.0.1' detected across referenced global_ips")
	})
}

// writeTempFile creates a temporary file with the given content and returns its path.
func writeTempFile(t *testing.T, content []byte) string {
	file, err := os.CreateTemp("", "test-*.pem")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer file.Close()
	if _, err := file.Write(content); err != nil {
		t.Fatalf("Failed to write to temp file: %v", err)
	}
	return file.Name()
}
