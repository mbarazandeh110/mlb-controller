package config

import "time"

// EnvoyConfig defines configuration for an Envoy load balancer.
type EnvoyConfig struct {
	Type               string            `mapstructure:"type"` // must be "envoy"
	Name               string            `mapstructure:"name"`
	IPReplacement      bool              `mapstructure:"ip_replacement"`
	IPReplacementList  IPReplacementList `mapstructure:"ip_replacement_list"`
	Addresses          []AddressConfig   `mapstructure:"addresses"`
	UpstreamSyncPeriod time.Duration     `mapstructure:"upstream_sync_period"`
	RequestTimeout     time.Duration     `mapstructure:"request_timeout"`
	Protocol           string            `mapstructure:"protocol"` // http|https|grpc
	Hostname           string            `mapstructure:"hostname,omitempty"`
	CertFile           string            `mapstructure:"cert_file,omitempty"`
	KeyFile            string            `mapstructure:"key_file,omitempty"`
	CAFile             string            `mapstructure:"ca_file,omitempty"`
	RequestPoolSize    int               `mapstructure:"request_pool_size"`
}

func (c EnvoyConfig) GetName() string                         { return c.Name }
func (c EnvoyConfig) GetAddresses() []AddressConfig           { return c.Addresses }
func (c EnvoyConfig) GetIPReplacement() bool                  { return c.IPReplacement }
func (c EnvoyConfig) GetIPReplacementList() IPReplacementList { return c.IPReplacementList }
func (c EnvoyConfig) GetType() string                         { return c.Type }
func (c EnvoyConfig) GetHostName() string                     { return c.Hostname }
func (c EnvoyConfig) GetProtocol() string                     { return c.Protocol }
func (c EnvoyConfig) GetCertFile() string                     { return c.CertFile }
func (c EnvoyConfig) GetKeyFile() string                      { return c.KeyFile }
func (c EnvoyConfig) GetCAFile() string                       { return c.CAFile }
func (c EnvoyConfig) GetRequestPoolSize() int                 { return c.RequestPoolSize }

func (c EnvoyConfig) SetDefaults(globalUpstreamSyncPeriod time.Duration, globalRequestPoolSize int) LoadBalancerConfig {
	if c.UpstreamSyncPeriod == 0 {
		c.UpstreamSyncPeriod = globalUpstreamSyncPeriod
	}
	if c.RequestTimeout == 0 {
		c.RequestTimeout = 30 * time.Second
	}
	if c.RequestPoolSize == 0 {
		c.RequestPoolSize = globalRequestPoolSize
	}
	return c
}
