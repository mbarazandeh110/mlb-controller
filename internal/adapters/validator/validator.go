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
			&RequestPoolValidator{},
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
		return fmt.Errorf("global_upstream_sync_period must be at least 1s, got %v", cfg.GlobalUpstreamSyncPeriod)
	}
	if cfg.GlobalUpstreamSyncPeriod < 0 {
		return fmt.Errorf("global_upstream_sync_period must be non-negative")
	}
	for _, lb := range cfg.LoadBalancers.LoadBalancers {
		if lb.GetRequestPoolSize() <= 0 { // After defaults
			return fmt.Errorf("loadbalancer '%s': request_pool_size must be positive (after fallback to global)", lb.GetName())
		}
	}
	return nil
}

type RequestPoolValidator struct{}

func (v *RequestPoolValidator) Validate(cfg *config.Config) error {
	if cfg.RequestPoolSize <= 0 {
		return fmt.Errorf("request_pool_size must be positive")
	}
	return nil
}
