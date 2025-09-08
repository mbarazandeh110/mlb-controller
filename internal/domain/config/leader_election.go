package config

import (
	"time"
)

// LeaderElectionConfig defines settings for leader election.
type LeaderElectionConfig struct {
	Enabled        bool          `mapstructure:"enabled"`
	LeaseName      string        `mapstructure:"lease_name"`
	LeaseNamespace string        `mapstructure:"lease_namespace"`
	LeaseDuration  time.Duration `mapstructure:"lease_duration"`
	RenewDeadline  time.Duration `mapstructure:"renew_deadline"`
	RetryPeriod    time.Duration `mapstructure:"retry_period"`
}
