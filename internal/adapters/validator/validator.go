package validator

import (
	"fmt"
	"mlb-controller/internal/domain/config"
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
