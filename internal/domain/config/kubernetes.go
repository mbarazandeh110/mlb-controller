package config

import (
	"time"
)

// KubernetesConfig defines Kubernetes client configuration.
type KubernetesConfig struct {
	ResyncPeriod     time.Duration `mapstructure:"resync_period"`
	KubernetesConfig string        `mapstructure:"kubernetes_config"`
}
