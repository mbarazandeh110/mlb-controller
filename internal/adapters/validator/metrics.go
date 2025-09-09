package validator

import (
	"fmt"
	domain "mlb-controller/internal/domain/config"
)

// MetricsValidator validates metrics configuration.
type MetricsValidator struct{}

func (v *MetricsValidator) Validate(cfg *domain.Config) error {
	if !cfg.Metrics.Enabled {
		return nil
	}
	if cfg.Metrics.Port < 0 || cfg.Metrics.Port > 65535 {
		return fmt.Errorf("metrics.port must be between 0 and 65535")
	}
	if cfg.Metrics.URI == "" {
		return fmt.Errorf("metrics.uri is required when enabled")
	}
	return nil
}
