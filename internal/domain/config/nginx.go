package config

import "time"

// NginxConfig defines configuration for an Nginx load balancer.
type NginxConfig struct {
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
}

func (c NginxConfig) GetName() string                         { return c.Name }
func (c NginxConfig) GetAddresses() []AddressConfig           { return c.Addresses }
func (c NginxConfig) GetIPReplacement() bool                  { return c.IPReplacement }
func (c NginxConfig) GetIPReplacementList() IPReplacementList { return c.IPReplacementList }
