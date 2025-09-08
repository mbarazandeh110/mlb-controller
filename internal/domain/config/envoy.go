package config

import "time"

// EnvoyConfig defines configuration for an Envoy load balancer.
type EnvoyConfig struct {
	Name               string            `mapstructure:"name"`
	IPReplacement      bool              `mapstructure:"ip_replacement"`
	IPReplacementList  IPReplacementList `mapstructure:"ip_replacement_list"`
	Addresses          []AddressConfig   `mapstructure:"addresses"`
	UpstreamSyncPeriod time.Duration     `mapstructure:"upstream_sync_period"`
	RequestTimeout     time.Duration     `mapstructure:"request_timeout"`
}

func (c EnvoyConfig) GetName() string                         { return c.Name }
func (c EnvoyConfig) GetAddresses() []AddressConfig           { return c.Addresses }
func (c EnvoyConfig) GetIPReplacement() bool                  { return c.IPReplacement }
func (c EnvoyConfig) GetIPReplacementList() IPReplacementList { return c.IPReplacementList }
