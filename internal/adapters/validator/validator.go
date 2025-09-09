package validator

import (
	"fmt"
	"mlb-controller/internal/domain/config"
	config_ports "mlb-controller/internal/ports/config"
	"mlb-controller/internal/util"
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
func (cv *CompositeValidator) Validate(cfg *config.Config) error {
	// Apply default values
	config.ApplyDefaultValues(cfg)

	for _, v := range cv.validators {
		if err := v.Validate(cfg); err != nil {
			return fmt.Errorf("validation failed: %w", err)
		}
	}
	return nil
}

// GlobalConfigValidator validates global configuration settings.
type GlobalConfigValidator struct{}

func (v *GlobalConfigValidator) Validate(cfg *config.Config) error {
	if cfg.GlobalUpstreamSyncPeriod < 0 {
		return fmt.Errorf("global_upstream_sync_period must be non-negative")
	}
	return nil
}

// LeaderElectionValidator validates leader election configuration.
type LeaderElectionValidator struct{}

func (v *LeaderElectionValidator) Validate(cfg *config.Config) error {
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

func (v *LoadBalancersValidator) Validate(cfg *config.Config) error {
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
func validateLoadBalancer(lb config.LoadBalancerConfig, globalList config.GlobalIPReplacementList) error {
	// Validate addresses
	for _, addr := range lb.GetAddresses() {
		if !util.IsValidIP(addr.IP) {
			return fmt.Errorf("invalid IP in addresses: %s", addr.IP)
		}
		if !util.IsValidPort(addr.Port) {
			return fmt.Errorf("port must be between 0 and 65535: %d", addr.Port)
		}
		if !util.IsValidProtocol(addr.Protocol) {
			return fmt.Errorf("protocol must be one of: http, https, grpc; got: %s", addr.Protocol)
		}
		if addr.Protocol == "https" && addr.Hostname == "" {
			return fmt.Errorf("hostname is required for https protocol")
		}
		if addr.Hostname != "" && !util.IsValidDomain(addr.Hostname) {
			return fmt.Errorf("invalid hostname: %s", addr.Hostname)
		}
	}

	// Validate IP replacement list
	if lb.GetIPReplacement() {
		if err := util.ValidateIPReplacementList(lb.GetIPReplacementList(), globalList); err != nil {
			return err
		}
	}

	return nil
}
