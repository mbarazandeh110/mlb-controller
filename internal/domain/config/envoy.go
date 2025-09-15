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
	Protocol           string            `mapstructure:"protocol"` // grpc
	Hostname           string            `mapstructure:"hostname,omitempty"`
	CertPath           string            `mapstructure:"cert_path,omitempty"`
	KeyPath            string            `mapstructure:"key_path,omitempty"`
	CAPath             string            `mapstructure:"ca_path,omitempty"`
	Cert               []byte            `mapstructure:"-"` // This will hold the content
	Key                []byte            `mapstructure:"-"` // This will hold the content
	CA                 []byte            `mapstructure:"-"` // This will hold the content
	RequestPoolSize    int               `mapstructure:"request_pool_size"`
}

func (c EnvoyConfig) GetName() string                         { return c.Name }
func (c EnvoyConfig) GetAddresses() []AddressConfig           { return c.Addresses }
func (c EnvoyConfig) GetIPReplacement() bool                  { return c.IPReplacement }
func (c EnvoyConfig) GetIPReplacementList() IPReplacementList { return c.IPReplacementList }
func (c EnvoyConfig) GetType() string                         { return c.Type }
func (c EnvoyConfig) GetHostName() string                     { return c.Hostname }
func (c EnvoyConfig) GetProtocol() string                     { return c.Protocol }
func (c EnvoyConfig) GetCertPath() string                     { return c.CertPath }
func (c EnvoyConfig) GetKeyPath() string                      { return c.KeyPath }
func (c EnvoyConfig) GetCAPath() string                       { return c.CAPath }
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
