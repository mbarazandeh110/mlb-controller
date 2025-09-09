package validator

import (
	"fmt"

	domain "mlb-controller/internal/domain/config"
	config_ports "mlb-controller/internal/ports/config"
)

// CompositeValidator aggregates multiple validators.
type CompositeValidator struct {
	validators []config_ports.Validator
}

// NewCompositeValidator creates a new CompositeValidator with default validators.
func NewCompositeValidator() *CompositeValidator {
	return &CompositeValidator{
		validators: []config_ports.Validator{
			&GlobalConfigValidator{},
			&LeaderElectionValidator{},
			&LogValidator{},
			&MetricsValidator{},
			&KubernetesValidator{},
			&GlobalIPReplacementValidator{},
			&LoadBalancerValidator{},
			&NginxValidator{},
			&EnvoyValidator{},
		},
	}
}

// Validate runs all registered validators.
func (cv *CompositeValidator) Validate(cfg *domain.Config) error {
	// Apply default values
	domain.ApplyDefaultValues(cfg)

	for _, v := range cv.validators {
		if err := v.Validate(cfg); err != nil {
			return fmt.Errorf("validation failed: %w", err)
		}
	}
	return nil
}

// GlobalConfigValidator validates global configuration settings.
type GlobalConfigValidator struct{}

func (v *GlobalConfigValidator) Validate(cfg *domain.Config) error {
	if cfg.GlobalUpstreamSyncPeriod < 0 {
		return fmt.Errorf("global_upstream_sync_period must be non-negative")
	}
	return nil
}

// LeaderElectionValidator validates leader election configuration.
type LeaderElectionValidator struct{}

func (v *LeaderElectionValidator) Validate(cfg *domain.Config) error {
	if !cfg.LeaderElection.Enabled {
		return nil
	}
	if cfg.LeaderElection.LeaseName == "" {
		return fmt.Errorf("leader_election.lease_name is required when enabled")
	}
	if cfg.LeaderElection.LeaseNamespace == "" {
		return fmt.Errorf("leader_election.lease_namespace is required when enabled")
	}
	if cfg.LeaderElection.LeaseDuration <= 0 {
		return fmt.Errorf("leader_election.lease_duration must be positive")
	}
	if cfg.LeaderElection.RenewDeadline <= 0 {
		return fmt.Errorf("leader_election.renew_deadline must be positive")
	}
	if cfg.LeaderElection.RetryPeriod <= 0 {
		return fmt.Errorf("leader_election.retry_period must be positive")
	}
	return nil
}

// LoadBalancersValidator validates Nginx and Envoy configurations.
type LoadBalancersValidator struct{}

func (v *LoadBalancersValidator) Validate(cfg *domain.Config) error {
	// Validate unique names across all load balancers
	names := make(map[string]struct{})
	for _, lb := range cfg.LoadBalancers.Nginx {
		if lb.Name == "" {
			return fmt.Errorf("loadbalancers.nginx.name is required")
		}
		if _, exists := names[lb.Name]; exists {
			return fmt.Errorf("loadbalancers.nginx.name '%s' must be unique", lb.Name)
		}
		names[lb.Name] = struct{}{}
	}
	for _, lb := range cfg.LoadBalancers.Envoy {
		if lb.Name == "" {
			return fmt.Errorf("loadbalancers.envoy.name is required")
		}
		if _, exists := names[lb.Name]; exists {
			return fmt.Errorf("loadbalancers.envoy.name '%s' must be unique", lb.Name)
		}
		names[lb.Name] = struct{}{}
	}

	// Validate Nginx
	for _, lb := range cfg.LoadBalancers.Nginx {
		if err := validateLoadBalancer(lb, cfg.GlobalIPReplacementList); err != nil {
			return fmt.Errorf("loadbalancers.nginx '%s': %w", lb.Name, err)
		}
	}

	// Validate Envoy
	for _, lb := range cfg.LoadBalancers.Envoy {
		if err := validateLoadBalancer(lb, cfg.GlobalIPReplacementList); err != nil {
			return fmt.Errorf("loadbalancers.envoy '%s': %w", lb.Name, err)
		}
	}

	return nil
}

// Helper function to validate a load balancer configuration
func validateLoadBalancer(lb domain.LoadBalancerConfig, globalList domain.GlobalIPReplacementList) error {
	// Validate addresses
	for _, addr := range lb.GetAddresses() {
		if !isValidIP(addr.IP) {
			return fmt.Errorf("invalid IP in addresses: %s", addr.IP)
		}
		if addr.Port < 0 || addr.Port > 65535 {
			return fmt.Errorf("port must be between 0 and 65535: %d", addr.Port)
		}
		if !isValidProtocol(addr.Protocol) {
			return fmt.Errorf("protocol must be one of: http, https, grpc; got: %s", addr.Protocol)
		}
		if addr.Protocol == "https" && addr.Hostname == "" {
			return fmt.Errorf("hostname is required for https protocol")
		}
		if addr.Hostname != "" && !isValidDomain(addr.Hostname) {
			return fmt.Errorf("invalid hostname: %s", addr.Hostname)
		}
	}

	// Validate IP replacement list
	if lb.GetIPReplacement() {
		sourceNets := make(map[string]struct{})
		for _, net := range lb.GetIPReplacementList().Nets {
			if !isValidIP(net.Source) || !isValidIP(net.Target) {
				return fmt.Errorf("invalid IP in ip_replacement_list.nets: source=%s, target=%s", net.Source, net.Target)
			}
			if net.Mask < 0 || net.Mask > 32 {
				return fmt.Errorf("ip_replacement_list.nets.mask must be between 0 and 32")
			}
			sourceNet := fmt.Sprintf("%s/%d", net.Source, net.Mask)
			if _, exists := sourceNets[sourceNet]; exists {
				return fmt.Errorf("ip_replacement_list.nets source '%s' must not overlap", sourceNet)
			}
			sourceNets[sourceNet] = struct{}{}
		}

		sourceIPs := make(map[string]struct{})
		for _, ip := range lb.GetIPReplacementList().IPs {
			if !isValidIP(ip.Source) || !isValidIP(ip.Target) {
				return fmt.Errorf("invalid IP in ip_replacement_list.ips: source=%s, target=%s", ip.Source, ip.Target)
			}
			if _, exists := sourceIPs[ip.Source]; exists {
				return fmt.Errorf("ip_replacement_list.ips source '%s' must be unique", ip.Source)
			}
			sourceIPs[ip.Source] = struct{}{}
		}

		// Validate global references
		globalNetNames := make(map[string]struct{})
		for _, net := range globalList.Net {
			globalNetNames[net.Name] = struct{}{}
		}
		for _, name := range lb.GetIPReplacementList().GlobalNets {
			if _, exists := globalNetNames[name]; !exists {
				return fmt.Errorf("global_nets '%s' does not exist in global_ip_replacement_list.net", name)
			}
		}

		globalIPNames := make(map[string]struct{})
		for _, ip := range globalList.IP {
			globalIPNames[ip.Name] = struct{}{}
		}
		for _, name := range lb.GetIPReplacementList().GlobalIPs {
			if _, exists := globalIPNames[name]; !exists {
				return fmt.Errorf("global_ips '%s' does not exist in global_ip_replacement_list.ip", name)
			}
		}
	}

	return nil
}
