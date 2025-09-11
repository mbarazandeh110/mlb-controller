package validator

import (
	"fmt"
	"mlb-controller/internal/domain/config"
)

// EnvoyValidator validates Envoy-specific configuration.
type EnvoyValidator struct{}

func (v *EnvoyValidator) Validate(cfg *config.Config) error {
	for _, lb := range cfg.LoadBalancers.LoadBalancers {
		if envoy, ok := lb.(config.EnvoyConfig); ok {
			if err := v.validateEnvoyConfig(envoy); err != nil {
				return fmt.Errorf("envoy '%s': %w", envoy.Name, err)
			}
		}
	}
	return nil
}

func (v *EnvoyValidator) validateEnvoyConfig(lb config.EnvoyConfig) error {
	// Validate timeouts
	if lb.UpstreamSyncPeriod < 0 {
		return fmt.Errorf("upstream_sync_period must be non-negative")
	}
	if lb.RequestTimeout < 0 {
		return fmt.Errorf("request_timeout must be non-negative")
	}

	return nil
}
