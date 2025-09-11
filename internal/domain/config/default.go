package config

import "time"

// ApplyDefaultValues sets default values for the configuration.
func ApplyDefaultValues(cfg *Config) {
	// Initialize nil slices to empty slices
	if cfg.LoadBalancers.LoadBalancers == nil {
		cfg.LoadBalancers.LoadBalancers = []LoadBalancerConfig{}
	}

	if cfg.GlobalUpstreamSyncPeriod == 0 {
		cfg.GlobalUpstreamSyncPeriod = 10 * time.Second
	}
	if cfg.LeaderElection.LeaseDuration == 0 {
		cfg.LeaderElection.LeaseDuration = 15 * time.Second
	}
	if cfg.LeaderElection.RenewDeadline == 0 {
		cfg.LeaderElection.RenewDeadline = 10 * time.Second
	}
	if cfg.LeaderElection.RetryPeriod == 0 {
		cfg.LeaderElection.RetryPeriod = 2 * time.Second
	}
	if cfg.Log.Level == "" {
		cfg.Log.Level = "info"
	}
	if cfg.Log.Format == "" {
		cfg.Log.Format = "json"
	}
	if cfg.Metrics.Port == 0 {
		cfg.Metrics.Port = 9090
	}
	if cfg.Metrics.URI == "" {
		cfg.Metrics.URI = "/metrics"
	}
	if cfg.Kubernetes.ResyncPeriod == 0 {
		cfg.Kubernetes.ResyncPeriod = 30 * time.Second
	}

	for i, lb := range cfg.LoadBalancers.LoadBalancers {
		switch v := lb.(type) {
		case NginxConfig:
			if v.UpstreamSyncPeriod == 0 {
				v.UpstreamSyncPeriod = cfg.GlobalUpstreamSyncPeriod
			}
			if v.FailTimeout == 0 {
				v.FailTimeout = 60 * time.Second
			}
			if v.RequestTimeout == 0 {
				v.RequestTimeout = 30 * time.Second
			}
			cfg.LoadBalancers.LoadBalancers[i] = v
		case EnvoyConfig:
			if v.UpstreamSyncPeriod == 0 {
				v.UpstreamSyncPeriod = cfg.GlobalUpstreamSyncPeriod
			}
			if v.RequestTimeout == 0 {
				v.RequestTimeout = 30 * time.Second
			}
			cfg.LoadBalancers.LoadBalancers[i] = v
		}
	}
}
