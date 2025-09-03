package config

import (
	"fmt"
	"net"
	"regexp"
	"time"

	"github.com/spf13/viper"
)

// ============================
// Defaults
// ============================

// Constants for default values
const (
	defaultLeaderElectionEnabled    = false
	defaultGlobalUpstreamSyncPeriod = 10 * time.Second
	defaultLeaseDuration            = 15 * time.Second
	defaultRenewDeadline            = 10 * time.Second
	defaultRetryPeriod              = 2 * time.Second

	defaultLogEnabled = false
	defaultLogLevel   = "info"
	defaultLogFormat  = "json"

	defaultMetricsEnabled = false
	defaultMetricsPort    = 9090
	defaultMetricsURI     = "/metrics"

	defaultKubernetesResyncPeriod = 30 * time.Second

	defaultFailTimeout    = 60 * time.Second
	defaultRequestTimeout = 30 * time.Second
)

// domainRegex is used to validate hostname as a valid domain name
var domainRegex = regexp.MustCompile(`^([a-zA-Z0-9]([a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?\.)+[a-zA-Z]{2,}$`)

// ============================
// Root Config
// ============================

// Config represents the entire configuration structure
type Config struct {
	GlobalUpstreamSyncPeriod time.Duration           `mapstructure:"global_upstream_sync_period"`
	LeaderElection           LeaderElectionConfig    `mapstructure:"leader_election"`
	Log                      LogConfig               `mapstructure:"log"`
	Metrics                  MetricsConfig           `mapstructure:"metrics"`
	Kubernetes               KubernetesConfig        `mapstructure:"kubernetes"`
	GlobalIPReplacementList  GlobalIPReplacementList `mapstructure:"global_ip_replacement_list"`
	LoadBalancers            LoadBalancersConfig     `mapstructure:"loadbalancers"`
}

// ============================
// Sub-configs
// ============================

// LeaderElectionConfig holds leader election settings
type LeaderElectionConfig struct {
	Enabled        bool          `mapstructure:"enabled"`
	LeaseName      string        `mapstructure:"lease_name"`
	LeaseNamespace string        `mapstructure:"lease_namespace"`
	LeaseDuration  time.Duration `mapstructure:"lease_duration"`
	RenewDeadline  time.Duration `mapstructure:"renew_deadline"`
	RetryPeriod    time.Duration `mapstructure:"retry_period"`
}

// LogConfig holds logging settings
type LogConfig struct {
	Enabled bool   `mapstructure:"enabled"`
	Level   string `mapstructure:"level"`  // debug, info, warn, error, fatal
	Format  string `mapstructure:"format"` // json, console
}

// MetricsConfig holds metrics settings
type MetricsConfig struct {
	Enabled bool   `mapstructure:"enabled"`
	Port    int    `mapstructure:"port"`
	URI     string `mapstructure:"uri"`
}

// KubernetesConfig holds Kubernetes settings
type KubernetesConfig struct {
	ResyncPeriod     time.Duration `mapstructure:"resync_period"`
	KubernetesConfig string        `mapstructure:"kubernetes_config"`
}

// ============================
// Replacement Lists
// ============================

// GlobalIPReplacementList holds global IP replacement rules
type GlobalIPReplacementList struct {
	Net []GlobalNetReplacement `mapstructure:"net"`
	IP  []GlobalIPReplacement  `mapstructure:"ip"`
}

// GlobalNetReplacement holds global network replacement rules
type GlobalNetReplacement struct {
	Name string      `mapstructure:"name"`
	Nets []NetConfig `mapstructure:"nets"`
}

// GlobalIPReplacement holds global IP replacement rules
type GlobalIPReplacement struct {
	Name string     `mapstructure:"name"`
	IPs  []IPConfig `mapstructure:"ips"`
}

// NetConfig holds network replacement configuration
type NetConfig struct {
	Source string `mapstructure:"source"`
	Target string `mapstructure:"target"`
	Mask   int    `mapstructure:"mask"`
}

// IPConfig holds IP replacement configuration
type IPConfig struct {
	Source string `mapstructure:"source"`
	Target string `mapstructure:"target"`
}

// ============================
// Load balancers
// ============================

// AddressConfig holds address configuration for load balancers
type AddressConfig struct {
	Protocol string `mapstructure:"protocol"` // http|https|grpc
	IP       string `mapstructure:"ip"`
	Port     int    `mapstructure:"port"`
	Hostname string `mapstructure:"hostname,omitempty"` // required if https
}

// LoadBalancersConfig holds load balancer configurations
type LoadBalancersConfig struct {
	Nginx []NginxConfig `mapstructure:"nginx"`
	Envoy []EnvoyConfig `mapstructure:"envoy"`
}

// LoadBalancerConfig defines a common interface for all load balancer configs
type LoadBalancerConfig interface {
	GetName() string
	GetAddresses() []AddressConfig
	GetIPReplacement() bool
	GetIPReplacementList() IPReplacementList
}

// NginxConfig holds Nginx-specific configuration
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

// Implement LoadBalancerConfig for NginxConfig
func (c NginxConfig) GetName() string                         { return c.Name }
func (c NginxConfig) GetAddresses() []AddressConfig           { return c.Addresses }
func (c NginxConfig) GetIPReplacement() bool                  { return c.IPReplacement }
func (c NginxConfig) GetIPReplacementList() IPReplacementList { return c.IPReplacementList }

// EnvoyConfig holds Envoy-specific configuration
type EnvoyConfig struct {
	Name               string            `mapstructure:"name"`
	IPReplacement      bool              `mapstructure:"ip_replacement"`
	IPReplacementList  IPReplacementList `mapstructure:"ip_replacement_list"`
	Addresses          []AddressConfig   `mapstructure:"addresses"`
	UpstreamSyncPeriod time.Duration     `mapstructure:"upstream_sync_period"`
	RequestTimeout     time.Duration     `mapstructure:"request_timeout"`
}

// Implement LoadBalancerConfig for EnvoyConfig
func (c EnvoyConfig) GetName() string                         { return c.Name }
func (c EnvoyConfig) GetAddresses() []AddressConfig           { return c.Addresses }
func (c EnvoyConfig) GetIPReplacement() bool                  { return c.IPReplacement }
func (c EnvoyConfig) GetIPReplacementList() IPReplacementList { return c.IPReplacementList }

// IPReplacementList holds IP replacement rules for load balancers
type IPReplacementList struct {
	Nets       []NetConfig `mapstructure:"nets"`
	IPs        []IPConfig  `mapstructure:"ips"`
	GlobalNets []string    `mapstructure:"global_nets"`
	GlobalIPs  []string    `mapstructure:"global_ips"`
}

// Loader: responsible for reading the file, applying defaults, and unmarshalling
type Loader struct {
	v *viper.Viper
}

func NewLoader(path string) *Loader {
	v := viper.New()
	v.SetConfigFile(path)
	setDefaults(v)
	return &Loader{v: v}
}

func (l *Loader) Load() (*Config, error) {
	// defaultLoggerCfg := logging.LogConfig{
	// 	Level:  "info",
	// 	Format: "json",
	// }
	// var log ports.Logger
	// log, _ = logging.New(defaultLoggerCfg)
	if err := l.v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("read config: %w", err)
		// log.Fatal("read conig", ports.Field{Key: "erro", Value: err})
	}

	cfg := &Config{}
	if err := l.v.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}

	// After Unmarshal, minimize zero values to default (this is the right place, not in Validate)
	normalizeWithDefaults(cfg)

	// Validation with a chain of Validators
	if err := defaultValidator().Validate(cfg); err != nil {
		return nil, fmt.Errorf("validate config: %w", err)
	}

	return cfg, nil
}

// Wrapper for compatibility with the previous API
func Load(path string) (*Config, error) {
	return NewLoader(path).Load()
}

// -------------------------
// Defaults & Normalize
// -------------------------

func setDefaults(v *viper.Viper) {
	v.SetDefault("global_upstream_sync_period", defaultGlobalUpstreamSyncPeriod)

	v.SetDefault("leader_election.enabled", defaultLeaderElectionEnabled)
	v.SetDefault("leader_election.lease_duration", defaultLeaseDuration)
	v.SetDefault("leader_election.renew_deadline", defaultRenewDeadline)
	v.SetDefault("leader_election.retry_period", defaultRetryPeriod)

	v.SetDefault("log.enabled", defaultLogEnabled)
	v.SetDefault("log.level", defaultLogLevel)
	v.SetDefault("log.format", defaultLogFormat)

	v.SetDefault("metrics.enabled", defaultMetricsEnabled)
	v.SetDefault("metrics.port", defaultMetricsPort)
	v.SetDefault("metrics.uri", defaultMetricsURI)

	v.SetDefault("kubernetes.resync_period", defaultKubernetesResyncPeriod)
}

// normalizeWithDefaults: fills any zero values in sub-configs with the appropriate default
func normalizeWithDefaults(c *Config) {
	// Nginx
	for i := range c.LoadBalancers.Nginx {
		if c.LoadBalancers.Nginx[i].UpstreamSyncPeriod == 0 {
			c.LoadBalancers.Nginx[i].UpstreamSyncPeriod = c.GlobalUpstreamSyncPeriod
		}
		if c.LoadBalancers.Nginx[i].FailTimeout == 0 {
			c.LoadBalancers.Nginx[i].FailTimeout = defaultFailTimeout
		}
		if c.LoadBalancers.Nginx[i].RequestTimeout == 0 {
			c.LoadBalancers.Nginx[i].RequestTimeout = defaultRequestTimeout
		}
	}

	// Envoy
	for i := range c.LoadBalancers.Envoy {
		if c.LoadBalancers.Envoy[i].UpstreamSyncPeriod == 0 {
			c.LoadBalancers.Envoy[i].UpstreamSyncPeriod = c.GlobalUpstreamSyncPeriod
		}
		if c.LoadBalancers.Envoy[i].RequestTimeout == 0 {
			c.LoadBalancers.Envoy[i].RequestTimeout = defaultRequestTimeout
		}
	}
}

// ============================
// Validator Interfaces
// ============================

type Validator interface {
	Validate(cfg *Config) error
}

type CompositeValidator struct {
	validators []Validator
}

func (cv *CompositeValidator) Validate(cfg *Config) error {
	for _, v := range cv.validators {
		if err := v.Validate(cfg); err != nil {
			return err
		}
	}
	return nil
}

func defaultValidator() Validator {
	return &CompositeValidator{
		validators: []Validator{
			&leaderElectionValidator{},
			&logValidator{},
			&ipReplacementValidator{},
			&loadBalancerValidator{},
		},
	}
}

// ============================
// Individual Validators
// ============================

type leaderElectionValidator struct{}

// Validate LeaderElection
func (v *leaderElectionValidator) Validate(c *Config) error {
	if c.LeaderElection.Enabled {
		if c.LeaderElection.LeaseName == "" {
			return fmt.Errorf("leader_election.lease_name is required when leader_election.enabled is true")
		}
		if c.LeaderElection.LeaseNamespace == "" {
			return fmt.Errorf("leader_election.lease_namespace is required when leader_election.enabled is true")
		}
	}
	return nil
}

type logValidator struct{}

// Validate validates the log configuration
func (v *logValidator) Validate(c *Config) error {
	if !c.Log.Enabled {
		return nil
	}
	// Validate Log level
	switch c.Log.Level {
	case "debug", "info", "warn", "error", "fatal":
	default:
		return fmt.Errorf("log.level must be one of: debug, info, warn, error, fatal")
	}
	switch c.Log.Format {
	// Validate Log format
	case "json", "console":
	default:
		return fmt.Errorf("log.format must be one of: console, json")
	}
	return nil
}

type ipReplacementValidator struct{}

// validateIPConfigs validates IP and Net configurations
func (v *ipReplacementValidator) Validate(c *Config) error {
	// Check for duplicate names in global_ip_replacement_list
	names := map[string]bool{}
	for _, g := range c.GlobalIPReplacementList.Net {
		if names[g.Name] {
			return fmt.Errorf("duplicate name %s in global_ip_replacement_list", g.Name)
		}
		names[g.Name] = true
	}
	for _, g := range c.GlobalIPReplacementList.IP {
		if names[g.Name] {
			return fmt.Errorf("duplicate name %s in global_ip_replacement_list", g.Name)
		}
		names[g.Name] = true
	}

	// Validate Global IP Replacement List
	for _, g := range c.GlobalIPReplacementList.Net {
		for _, n := range g.Nets {
			if err := validateNetConfig(n); err != nil {
				return fmt.Errorf("invalid net config in global_ip_replacement_list.net.%s: %w", g.Name, err)
			}
		}
	}
	for _, g := range c.GlobalIPReplacementList.IP {
		for _, ip := range g.IPs {
			if err := validateIPConfig(ip); err != nil {
				return fmt.Errorf("invalid ip config in global_ip_replacement_list.ip.%s: %w", g.Name, err)
			}
		}
	}

	// Check for CIDR overlap in global nets
	if err := checkCIDROverlap(c.GlobalIPReplacementList.Net); err != nil {
		return err
	}
	return nil
}

type loadBalancerValidator struct{}

func (v *loadBalancerValidator) Validate(c *Config) error {
	// duplicate names in loadbalancers
	names := map[string]bool{}
	for _, n := range c.LoadBalancers.Nginx {
		if names[n.Name] {
			return fmt.Errorf("duplicate name %s in loadbalancers", n.Name)
		}
		names[n.Name] = true
	}
	for _, e := range c.LoadBalancers.Envoy {
		if names[e.Name] {
			return fmt.Errorf("duplicate name %s in loadbalancers", e.Name)
		}
		names[e.Name] = true
	}

	// duplicate IPs across all addresses
	ips := map[string]bool{}
	for _, n := range c.LoadBalancers.Nginx {
		for _, a := range n.Addresses {
			if ips[a.IP] {
				return fmt.Errorf("duplicate address %s in loadbalancers", a.IP)
			}
			ips[a.IP] = true
		}
	}
	for _, e := range c.LoadBalancers.Envoy {
		for _, a := range e.Addresses {
			if ips[a.IP] {
				return fmt.Errorf("duplicate address %s in loadbalancers", a.IP)
			}
			ips[a.IP] = true
		}
	}

	// validate each LB config using common validator
	for _, n := range c.LoadBalancers.Nginx {
		if err := validateLoadBalancerConfig(n, c.GlobalIPReplacementList); err != nil {
			return fmt.Errorf("invalid nginx config for %s: %w", n.Name, err)
		}
	}
	for _, e := range c.LoadBalancers.Envoy {
		if err := validateLoadBalancerConfig(e, c.GlobalIPReplacementList); err != nil {
			return fmt.Errorf("invalid envoy config for %s: %w", e.Name, err)
		}
	}
	return nil
}

// ============================
// Low-level validators (pure functions)
// ============================

// validateLoadBalancerConfig validates any LoadBalancerConfig (Nginx, Envoy, …)
func validateLoadBalancerConfig(cfg LoadBalancerConfig, globalList GlobalIPReplacementList) error {
	if len(cfg.GetAddresses()) == 0 {
		return fmt.Errorf("addresses field is required and must contain at least one address for config %s", cfg.GetName())
	}
	for _, a := range cfg.GetAddresses() {
		if err := validateAddressConfig(a); err != nil {
			return fmt.Errorf("invalid address in config %s: %w", cfg.GetName(), err)
		}
	}
	if cfg.GetIPReplacement() {
		return validateIPReplacementList(cfg.GetIPReplacementList(), globalList, cfg.GetName())
	}
	return nil
}

// validateAddressConfig validates a single AddressConfig
func validateAddressConfig(a AddressConfig) error {
	switch a.Protocol {
	case "http", "https", "grpc":
	default:
		return fmt.Errorf("invalid protocol %s, must be http, https, or grpc", a.Protocol)
	}
	if net.ParseIP(a.IP) == nil {
		return fmt.Errorf("invalid IP address %s", a.IP)
	}
	if a.Port <= 0 || a.Port > 65535 {
		return fmt.Errorf("invalid port %d, must be between 1 and 65535", a.Port)
	}
	if a.Protocol == "https" && a.Hostname == "" {
		return fmt.Errorf("hostname is required for https protocol")
	}
	if a.Hostname != "" && !domainRegex.MatchString(a.Hostname) {
		return fmt.Errorf("invalid hostname %s, must be a valid domain name", a.Hostname)
	}
	return nil
}

// validateIPReplacementList validates IP replacement lists
func validateIPReplacementList(list IPReplacementList, globalList GlobalIPReplacementList, lbName string) error {
	// Validate local nets and IPs
	for _, n := range list.Nets {
		if err := validateNetConfig(n); err != nil {
			return fmt.Errorf("invalid net in loadbalancer %s: %w", lbName, err)
		}
	}
	for _, i := range list.IPs {
		if err := validateIPConfig(i); err != nil {
			return fmt.Errorf("invalid ip in loadbalancer %s: %w", lbName, err)
		}
	}

	// Validate global net and IP references
	for _, gNet := range list.GlobalNets {
		if !globalNetExists(gNet, globalList.Net) {
			return fmt.Errorf("global net %s referenced in loadbalancer %s does not exist", gNet, lbName)
		}
	}
	for _, gIP := range list.GlobalIPs {
		if !globalIPExists(gIP, globalList.IP) {
			return fmt.Errorf("global ip %s referenced in loadbalancer %s does not exist", gIP, lbName)
		}
	}

	// Check for CIDR overlap including global nets
	var allNets []GlobalNetReplacement
	allNets = append(allNets, GlobalNetReplacement{Name: lbName, Nets: list.Nets})
	for _, name := range list.GlobalNets {
		for _, gn := range globalList.Net {
			if gn.Name == name {
				allNets = append(allNets, gn)
			}
		}
	}
	if err := checkCIDROverlap(allNets); err != nil {
		return fmt.Errorf("CIDR overlap in loadbalancer %s: %w", lbName, err)
	}

	// Check for CIDR overlap including global nets
	var allIPs []GlobalIPReplacement
	allIPs = append(allIPs, GlobalIPReplacement{Name: lbName, IPs: list.IPs})
	for _, name := range list.GlobalIPs {
		for _, gip := range globalList.IP {
			if gip.Name == name {
				allIPs = append(allIPs, gip)
			}
		}
	}

	if err := checkduplicateIP(allIPs); err != nil {
		return fmt.Errorf("duplicate ip in loadbalancer %s: %w", lbName, err)
	}
	return nil
}

// validateNetConfig validates a single NetConfig
func validateNetConfig(n NetConfig) error {
	if n.Mask < 0 || n.Mask > 32 {
		return fmt.Errorf("mask must be between 0 and 32: %d", n.Mask)
	}
	if _, _, err := net.ParseCIDR(fmt.Sprintf("%s/%d", n.Source, n.Mask)); err != nil {
		return fmt.Errorf("invalid source IP or mask: %s/%d: %w", n.Source, n.Mask, err)
	}

	if _, _, err := net.ParseCIDR(fmt.Sprintf("%s/%d", n.Target, n.Mask)); err != nil {
		return fmt.Errorf("invalid target IP or mask: %s/%d: %w", n.Target, n.Mask, err)
	}
	return nil
}

// validateIPConfig validates a single IPConfig
func validateIPConfig(i IPConfig) error {
	if net.ParseIP(i.Source) == nil {
		return fmt.Errorf("invalid source IP: %s", i.Source)
	}
	if net.ParseIP(i.Target) == nil {
		return fmt.Errorf("invalid target IP: %s", i.Target)
	}
	return nil
}

// globalNetExists checks if a global net exists
func globalNetExists(name string, nets []GlobalNetReplacement) bool {
	for _, n := range nets {
		if n.Name == name {
			return true
		}
	}
	return false
}

// globalIPExists checks if a global IP exists
func globalIPExists(name string, ips []GlobalIPReplacement) bool {
	for _, i := range ips {
		if i.Name == name {
			return true
		}
	}
	return false
}

// checkCIDROverlap checks for CIDR overlaps in a list of net replacements
func checkCIDROverlap(nets []GlobalNetReplacement) error {
	for i, net1 := range nets {
		for _, n1 := range net1.Nets {
			_, ipNet1, err := net.ParseCIDR(fmt.Sprintf("%s/%d", n1.Source, n1.Mask))
			if err != nil {
				return fmt.Errorf("invalid CIDR: %s/%d: %w", n1.Source, n1.Mask, err)
			}
			for j, net2 := range nets {
				if i == j {
					continue
				}
				for _, n2 := range net2.Nets {
					_, ipNet2, err := net.ParseCIDR(fmt.Sprintf("%s/%d", n2.Source, n2.Mask))
					if err != nil {
						return fmt.Errorf("invalid CIDR: %s/%d: %w", n2.Source, n2.Mask, err)
					}
					if ipNet1.Contains(ipNet2.IP) || ipNet2.Contains(ipNet1.IP) {
						return fmt.Errorf("CIDR overlap between %s/%d and %s/%d",
							n1.Source, n1.Mask, n2.Source, n2.Mask)
					}
				}
			}
		}
	}
	return nil
}

func checkduplicateIP(gips []GlobalIPReplacement) error {
	for i, ip1 := range gips {
		for _, i1 := range ip1.IPs {
			for j, ip2 := range gips {
				if i == j {
					continue
				}
				for _, i2 := range ip2.IPs {
					if i1.Source == i2.Source {
						return fmt.Errorf("duplicate source ip %s", i1.Source)
					}
					if i1.Target == i2.Target {
						return fmt.Errorf("duplicate target ip %s", i1.Target)
					}
				}
			}
		}
	}
	return nil
}
