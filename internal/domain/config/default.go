package config

import "time"

// ApplyDefaultValues sets default values for the configuration.
func ApplyDefaultValues(cfg *Config) {
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
	for i := range cfg.LoadBalancers.Nginx {
		if cfg.LoadBalancers.Nginx[i].UpstreamSyncPeriod == 0 {
			cfg.LoadBalancers.Nginx[i].UpstreamSyncPeriod = cfg.GlobalUpstreamSyncPeriod
		}
		if cfg.LoadBalancers.Nginx[i].FailTimeout == 0 {
			cfg.LoadBalancers.Nginx[i].FailTimeout = 60 * time.Second
		}
		if cfg.LoadBalancers.Nginx[i].RequestTimeout == 0 {
			cfg.LoadBalancers.Nginx[i].RequestTimeout = 30 * time.Second
		}
	}
	for i := range cfg.LoadBalancers.Envoy {
		if cfg.LoadBalancers.Envoy[i].UpstreamSyncPeriod == 0 {
			cfg.LoadBalancers.Envoy[i].UpstreamSyncPeriod = cfg.GlobalUpstreamSyncPeriod
		}
		if cfg.LoadBalancers.Envoy[i].RequestTimeout == 0 {
			cfg.LoadBalancers.Envoy[i].RequestTimeout = 30 * time.Second
		}
	}
}
