package validator

import (
	"fmt"
	"mlb-controller/internal/domain/config"
)

// NginxValidator validates Nginx-specific configuration.
type NginxValidator struct{}

func (v *NginxValidator) Validate(cfg *config.Config) error {
	for _, lb := range cfg.LoadBalancers.Nginx {
		if err := v.validateNginxConfig(lb); err != nil {
			return fmt.Errorf("nginx '%s': %w", lb.Name, err)
		}
	}
	return nil
}

func (v *NginxValidator) validateNginxConfig(lb config.NginxConfig) error {
	// Validate APIs
	if lb.ListAPI == "" {
		return fmt.Errorf("list_api is required")
	}
	if lb.AddAPI == "" {
		return fmt.Errorf("add_api is required")
	}
	if lb.RemoveAPI == "" {
		return fmt.Errorf("remove_api is required")
	}

	// Validate timeouts
	if lb.UpstreamSyncPeriod < 0 {
		return fmt.Errorf("upstream_sync_period must be non-negative")
	}
	if lb.FailTimeout < 0 {
		return fmt.Errorf("fail_timeout must be non-negative")
	}
	if lb.RequestTimeout < 0 {
		return fmt.Errorf("request_timeout must be non-negative")
	}

	return nil
}
