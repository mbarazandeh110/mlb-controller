package validator

import (
	"fmt"
	"mlb-controller/internal/domain/config"
	"mlb-controller/internal/util"
	"net/url"
	"strings"
)

// NginxValidator validates Nginx-specific configuration.
type NginxValidator struct{}

func (v *NginxValidator) Validate(cfg *config.Config) error {
	for _, lb := range cfg.LoadBalancers.LoadBalancers {
		if nginx, ok := lb.(config.NginxConfig); ok {
			if err := v.validateNginxConfig(nginx); err != nil {
				return fmt.Errorf("nginx '%s': %w", nginx.Name, err)
			}
		}
	}
	return nil
}

func (v *NginxValidator) validateNginxConfig(lb config.NginxConfig) error {
	// Validate APIs

	if lb.Protocol != "http" && lb.Protocol != "https" {
		return fmt.Errorf("loadbalancers.%s '%s': protocol must be one of: http, https, got: %s", lb.Type, lb.Name, lb.Protocol)
	}
	if lb.Protocol == "https" && !util.IsValidDomain(lb.Hostname) {
		if lb.Hostname == "" {
			return fmt.Errorf("loadbalancers.%s '%s': hostname is required for https protocol", lb.Type, lb.Name)
		}
		return fmt.Errorf("loadbalancers.%s '%s': invalid hostname: %s", lb.Type, lb.Name, lb.Hostname)
	}

	if lb.ListAPI == "" {
		return fmt.Errorf("list_api is required")
	}
	if lb.AddAPI == "" {
		return fmt.Errorf("add_api is required")
	}
	if lb.RemoveAPI == "" {
		return fmt.Errorf("remove_api is required")
	}

	if !v.isValidRelativeURI(lb.ListAPI) {
		return fmt.Errorf("list_api is invalid")
	}
	if !v.isValidRelativeURI(lb.AddAPI) {
		return fmt.Errorf("add_api is invalid")
	}
	if !v.isValidRelativeURI(lb.RemoveAPI) {
		return fmt.Errorf("remove_api is invalid")
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

func (v *NginxValidator) isValidRelativeURI(s string) bool {
	if !strings.HasPrefix(s, "/") {
		return false
	}

	parsed, err := url.Parse(s)
	if err != nil {
		return false
	}

	if parsed.Scheme != "" || parsed.Host != "" {
		return false
	}

	if parsed.Path == "" {
		return false
	}

	return true
}
