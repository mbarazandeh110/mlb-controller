package config

import (
	"fmt"
	"net"
	"regexp"
	"time"
)

// domainRegex برای ولیدیشن hostname
var domainRegex = regexp.MustCompile(`^([a-zA-Z0-9]([a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?\.)+[a-zA-Z]{2,}$`)

// ============================
// Root Config
// ============================

type Config struct {
	GlobalUpstreamSyncPeriod time.Duration           `mapstructure:"global_upstream_sync_period"`
	LeaderElection           LeaderElectionConfig    `mapstructure:"leader_election"`
	Log                      LogConfig               `mapstructure:"log"`
	Metrics                  MetricsConfig           `mapstructure:"metrics"`
	Kubernetes               KubernetesConfig        `mapstructure:"kubernetes"`
	GlobalIPReplacementList  GlobalIPReplacementList `mapstructure:"global_ip_replacement_list"`
	LoadBalancers            LoadBalancersConfig     `mapstructure:"loadbalancers"`
}

// --- Sub-configs ---
type LeaderElectionConfig struct {
	Enabled        bool          `mapstructure:"enabled"`
	LeaseName      string        `mapstructure:"lease_name"`
	LeaseNamespace string        `mapstructure:"lease_namespace"`
	LeaseDuration  time.Duration `mapstructure:"lease_duration"`
	RenewDeadline  time.Duration `mapstructure:"renew_deadline"`
	RetryPeriod    time.Duration `mapstructure:"retry_period"`
}

type LogConfig struct {
	Enabled bool   `mapstructure:"enabled"`
	Level   string `mapstructure:"level"`  // debug, info, warn, error, fatal
	Format  string `mapstructure:"format"` // json, console
}

type MetricsConfig struct {
	Enabled bool   `mapstructure:"enabled"`
	Port    int    `mapstructure:"port"`
	URI     string `mapstructure:"uri"`
}

type KubernetesConfig struct {
	ResyncPeriod     time.Duration `mapstructure:"resync_period"`
	KubernetesConfig string        `mapstructure:"kubernetes_config"`
}

// ============================
// Replacement Lists
// ============================

type GlobalIPReplacementList struct {
	Net []GlobalNetReplacement `mapstructure:"net"`
	IP  []GlobalIPReplacement  `mapstructure:"ip"`
}

type GlobalNetReplacement struct {
	Name string      `mapstructure:"name"`
	Nets []NetConfig `mapstructure:"nets"`
}

type GlobalIPReplacement struct {
	Name string     `mapstructure:"name"`
	IPs  []IPConfig `mapstructure:"ips"`
}

type NetConfig struct {
	Source string `mapstructure:"source"`
	Target string `mapstructure:"target"`
	Mask   int    `mapstructure:"mask"`
}

type IPConfig struct {
	Source string `mapstructure:"source"`
	Target string `mapstructure:"target"`
}

// ============================
// Load balancers
// ============================

type AddressConfig struct {
	Protocol string `mapstructure:"protocol"` // http|https|grpc
	IP       string `mapstructure:"ip"`
	Port     int    `mapstructure:"port"`
	Hostname string `mapstructure:"hostname,omitempty"`
}

type LoadBalancersConfig struct {
	Nginx []NginxConfig `mapstructure:"nginx"`
	Envoy []EnvoyConfig `mapstructure:"envoy"`
}

type LoadBalancerConfig interface {
	GetName() string
	GetAddresses() []AddressConfig
	GetIPReplacement() bool
	GetIPReplacementList() IPReplacementList
}

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

type IPReplacementList struct {
	Nets       []NetConfig `mapstructure:"nets"`
	IPs        []IPConfig  `mapstructure:"ips"`
	GlobalNets []string    `mapstructure:"global_nets"`
	GlobalIPs  []string    `mapstructure:"global_ips"`
}

// ============================
// Validation
// ============================

func (c *Config) Validate() error {
	// leader election rules
	if c.LeaderElection.Enabled {
		if c.LeaderElection.LeaseName == "" {
			return fmt.Errorf("leader_election.lease_name is required when enabled")
		}
		if c.LeaderElection.LeaseNamespace == "" {
			return fmt.Errorf("leader_election.lease_namespace is required when enabled")
		}
	}

	// log rules
	if c.Log.Enabled {
		switch c.Log.Level {
		case "debug", "info", "warn", "error", "fatal":
		default:
			return fmt.Errorf("log.level invalid: %s", c.Log.Level)
		}
		switch c.Log.Format {
		case "json", "console":
		default:
			return fmt.Errorf("log.format invalid: %s", c.Log.Format)
		}
	}

	// مثال: اعتبارسنجی AddressConfig
	for _, n := range c.LoadBalancers.Nginx {
		for _, a := range n.Addresses {
			if net.ParseIP(a.IP) == nil {
				return fmt.Errorf("invalid IP: %s", a.IP)
			}
			if a.Protocol == "https" && a.Hostname == "" {
				return fmt.Errorf("hostname required for https")
			}
			if a.Hostname != "" && !domainRegex.MatchString(a.Hostname) {
				return fmt.Errorf("invalid hostname: %s", a.Hostname)
			}
		}
	}

	return nil
}
