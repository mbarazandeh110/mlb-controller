package validator

import (
	"fmt"
	"mlb-controller/internal/domain/config"
)

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
