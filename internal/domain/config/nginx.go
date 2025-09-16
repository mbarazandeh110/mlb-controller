// internal/domain/config/nginx.go
package config

import "time"

// NginxConfig defines configuration for an Nginx load balancer.

// NginxConfig defines configuration for an Nginx load balancer.
type NginxConfig struct {
	Type               string            `mapstructure:"type"` // must be "nginx"
	Name               string            `mapstructure:"name"`
	IPReplacement      bool              `mapstructure:"ip_replacement"`
	IPReplacementList  IPReplacementList `mapstructure:"ip_replacement_list"`
	Addresses          []AddressConfig   `mapstructure:"addresses"`
	ListAPI            string            `mapstructure:"list_api"`
	AddAPI             string            `mapstructure:"add_api"`
	RemoveAPI          string            `mapstructure:"remove_api"`
	UpstreamSyncPeriod time.Duration     `mapstructure:"upstream_sync_period"`
	FailTimeout        time.Duration     `mapstructure:"fail_timeout"`
	RequestTimeout     time.Duration     `mapstructure:"request_timeout"`
	Protocol           string            `mapstructure:"protocol"` // http|https
	Hostname           string            `mapstructure:"hostname,omitempty"`
	CertPath           string            `mapstructure:"cert_path,omitempty"`
	KeyPath            string            `mapstructure:"key_path,omitempty"`
	CAPath             string            `mapstructure:"ca_path,omitempty"`
	Cert               []byte            `mapstructure:"-"` // This will hold the content
	Key                []byte            `mapstructure:"-"` // This will hold the content
	CA                 []byte            `mapstructure:"-"` // This will hold the content
	RequestPoolSize    int               `mapstructure:"request_pool_size"`
}

func (c NginxConfig) GetName() string                         { return c.Name }
func (c NginxConfig) GetAddresses() []AddressConfig           { return c.Addresses }
func (c NginxConfig) GetIPReplacement() bool                  { return c.IPReplacement }
func (c NginxConfig) GetIPReplacementList() IPReplacementList { return c.IPReplacementList }
func (c NginxConfig) GetType() string                         { return c.Type }
func (c NginxConfig) GetHostName() string                     { return c.Hostname }
func (c NginxConfig) GetProtocol() string                     { return c.Protocol }
func (c NginxConfig) GetCertPath() string                     { return c.CertPath }
func (c NginxConfig) GetKeyPath() string                      { return c.KeyPath }
func (c NginxConfig) GetCAPath() string                       { return c.CAPath }
func (c NginxConfig) GetRequestTimeOut() time.Duration        { return c.RequestTimeout }
func (c NginxConfig) GetRequestPoolSize() int                 { return c.RequestPoolSize }

func (c NginxConfig) SetDefaults(globalUpstreamSyncPeriod time.Duration, globalRequestPoolSize int) LoadBalancerConfig {
	if c.UpstreamSyncPeriod == 0 {
		c.UpstreamSyncPeriod = globalUpstreamSyncPeriod
	}
	if c.FailTimeout == 0 {
		c.FailTimeout = 60 * time.Second
	}
	if c.RequestTimeout == 0 {
		c.RequestTimeout = 30 * time.Second
	}
	if c.RequestPoolSize == 0 {
		c.RequestPoolSize = globalRequestPoolSize
	}
	return c
}
