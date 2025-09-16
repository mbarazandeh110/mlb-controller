package config

import "time"

// LoadBalancersConfig defines configurations for all load balancers.
type LoadBalancersConfig struct {
	LoadBalancers []LoadBalancerConfig `mapstructure:"loadbalancers"`
}

// LoadBalancerConfig defines the interface for load balancer configurations.
type LoadBalancerConfig interface {
	GetName() string
	GetAddresses() []AddressConfig
	GetIPReplacement() bool
	GetIPReplacementList() IPReplacementList
	GetType() string
	GetHostName() string
	GetProtocol() string
	GetCertPath() string
	GetKeyPath() string
	GetCAPath() string
	GetRequestPoolSize() int
	GetRequestTimeOut() time.Duration
	SetDefaults(globalUpstreamSyncPeriod time.Duration, globalRequestPoolSize int) LoadBalancerConfig
}

// AddressConfig defines an address for a load balancer.
type AddressConfig struct {
	IP   string `mapstructure:"ip"`
	Port int    `mapstructure:"port"`
}

// IPReplacementList defines IP and network replacement rules for a load balancer.
type IPReplacementList struct {
	Nets       []NetConfig `mapstructure:"nets"`
	IPs        []IPConfig  `mapstructure:"ips"`
	GlobalNets []string    `mapstructure:"global_nets"`
	GlobalIPs  []string    `mapstructure:"global_ips"`
}
