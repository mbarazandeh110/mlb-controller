package config

// LoadBalancersConfig defines configurations for Nginx and Envoy load balancers.
type LoadBalancersConfig struct {
	Nginx []NginxConfig `mapstructure:"nginx"`
	Envoy []EnvoyConfig `mapstructure:"envoy"`
}

// LoadBalancerConfig defines the interface for load balancer configurations.
type LoadBalancerConfig interface {
	GetName() string
	GetAddresses() []AddressConfig
	GetIPReplacement() bool
	GetIPReplacementList() IPReplacementList
}

// AddressConfig defines an address for a load balancer.
type AddressConfig struct {
	Protocol string `mapstructure:"protocol"` // http|https|grpc
	IP       string `mapstructure:"ip"`
	Port     int    `mapstructure:"port"`
	Hostname string `mapstructure:"hostname,omitempty"`
}

// IPReplacementList defines IP and network replacement rules for a load balancer.
type IPReplacementList struct {
	Nets       []NetConfig `mapstructure:"nets"`
	IPs        []IPConfig  `mapstructure:"ips"`
	GlobalNets []string    `mapstructure:"global_nets"`
	GlobalIPs  []string    `mapstructure:"global_ips"`
}
