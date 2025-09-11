package validator

import (
	"testing"
	"time"

	"mlb-controller/internal/domain/config"

	"github.com/stretchr/testify/assert"
)

func TestLeaderElectionValidator_Validate(t *testing.T) {
	tests := []struct {
		name        string
		config      *config.Config
		expectError bool
		errorMsg    string
	}{
		{
			name: "Valid Leader Election Config",
			config: &config.Config{
				LeaderElection: config.LeaderElectionConfig{
					Enabled:        true,
					LeaseName:      "leader-lease",
					LeaseNamespace: "default",
					LeaseDuration:  15 * time.Second,
					RenewDeadline:  10 * time.Second,
					RetryPeriod:    2 * time.Second,
				},
			},
			expectError: false,
		},
		{
			name: "Disabled Leader Election",
			config: &config.Config{
				LeaderElection: config.LeaderElectionConfig{
					Enabled:        false,
					LeaseName:      "", // Ignored because Enabled=false
					LeaseNamespace: "",
					LeaseDuration:  0,
					RenewDeadline:  0,
					RetryPeriod:    0,
				},
			},
			expectError: false,
		},
		{
			name: "Missing LeaseName",
			config: &config.Config{
				LeaderElection: config.LeaderElectionConfig{
					Enabled:        true,
					LeaseName:      "",
					LeaseNamespace: "default",
					LeaseDuration:  15 * time.Second,
					RenewDeadline:  10 * time.Second,
					RetryPeriod:    2 * time.Second,
				},
			},
			expectError: true,
			errorMsg:    "leader_election.lease_name is required when enabled",
		},
		{
			name: "Missing LeaseNamespace",
			config: &config.Config{
				LeaderElection: config.LeaderElectionConfig{
					Enabled:        true,
					LeaseName:      "leader-lease",
					LeaseNamespace: "",
					LeaseDuration:  15 * time.Second,
					RenewDeadline:  10 * time.Second,
					RetryPeriod:    2 * time.Second,
				},
			},
			expectError: true,
			errorMsg:    "leader_election.lease_namespace is required when enabled",
		},
		{
			name: "Non-positive LeaseDuration",
			config: &config.Config{
				LeaderElection: config.LeaderElectionConfig{
					Enabled:        true,
					LeaseName:      "leader-lease",
					LeaseNamespace: "default",
					LeaseDuration:  0,
					RenewDeadline:  10 * time.Second,
					RetryPeriod:    2 * time.Second,
				},
			},
			expectError: true,
			errorMsg:    "leader_election.lease_duration must be positive",
		},
		{
			name: "Non-positive RenewDeadline",
			config: &config.Config{
				LeaderElection: config.LeaderElectionConfig{
					Enabled:        true,
					LeaseName:      "leader-lease",
					LeaseNamespace: "default",
					LeaseDuration:  15 * time.Second,
					RenewDeadline:  0,
					RetryPeriod:    2 * time.Second,
				},
			},
			expectError: true,
			errorMsg:    "leader_election.renew_deadline must be positive",
		},
		{
			name: "Non-positive RetryPeriod",
			config: &config.Config{
				LeaderElection: config.LeaderElectionConfig{
					Enabled:        true,
					LeaseName:      "leader-lease",
					LeaseNamespace: "default",
					LeaseDuration:  15 * time.Second,
					RenewDeadline:  10 * time.Second,
					RetryPeriod:    0,
				},
			},
			expectError: true,
			errorMsg:    "leader_election.retry_period must be positive",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := &LeaderElectionValidator{}
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
