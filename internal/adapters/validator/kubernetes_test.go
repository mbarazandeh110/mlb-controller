package validator

import (
	"testing"
	"time"

	"mlb-controller/internal/domain/config"

	"github.com/stretchr/testify/assert"
)

func TestKubernetesValidator_Validate(t *testing.T) {
	tests := []struct {
		name        string
		config      *config.Config
		expectError bool
		errorMsg    string
	}{
		{
			name: "Valid ResyncPeriod",
			config: &config.Config{
				Kubernetes: config.KubernetesConfig{
					ResyncPeriod: 30 * time.Second,
				},
			},
			expectError: false,
		},
		{
			name: "Zero ResyncPeriod",
			config: &config.Config{
				Kubernetes: config.KubernetesConfig{
					ResyncPeriod: 0,
				},
			},
			expectError: false,
		},
		{
			name: "Negative ResyncPeriod",
			config: &config.Config{
				Kubernetes: config.KubernetesConfig{
					ResyncPeriod: -10 * time.Second,
				},
			},
			expectError: true,
			errorMsg:    "kubernetes.resync_period must be non-negative",
		},
		{
			name:        "Empty Config",
			config:      &config.Config{},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := &KubernetesValidator{}
			err := v.Validate(tt.config)

			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
