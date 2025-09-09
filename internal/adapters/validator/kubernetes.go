package validator

import (
	"fmt"
	domain "mlb-controller/internal/domain/config"
)

// KubernetesValidator validates Kubernetes configuration.
type KubernetesValidator struct{}

func (v *KubernetesValidator) Validate(cfg *domain.Config) error {
	if cfg.Kubernetes.ResyncPeriod < 0 {
		return fmt.Errorf("kubernetes.resync_period must be non-negative")
	}
	return nil
}
