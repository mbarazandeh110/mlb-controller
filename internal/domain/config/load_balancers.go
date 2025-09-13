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
	SetDefaults(globalUpstreamSyncPeriod time.Duration) LoadBalancerConfig
}

// AddressConfig defines an address for a load balancer.
type AddressConfig struct {
	Protocol string `mapstructure:"protocol"` // http|https|grpc
	IP       string `mapstructure:"ip"`
	Port     int    `mapstructure:"port"`
	Hostname string `mapstructure:"hostname,omitempty"`
	CertFile string `mapstructure:"certfile,omitempty"`
	KeyFile  string `mapstructure:"keyfile,omitempty"`
	CAFile   string `mapstructure:"ca_file,omitempty"`
}

// IPReplacementList defines IP and network replacement rules for a load balancer.
type IPReplacementList struct {
	Nets       []NetConfig `mapstructure:"nets"`
	IPs        []IPConfig  `mapstructure:"ips"`
	GlobalNets []string    `mapstructure:"global_nets"`
	GlobalIPs  []string    `mapstructure:"global_ips"`
}
