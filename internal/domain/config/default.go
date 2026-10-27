package config

import "time"

const (
	GlobalUpstreamSyncPeriod     = 10 * time.Second
	LeaderElectionLeaseDuration  = 15 * time.Second
	LeaderElectionRenewDeadline  = 10 * time.Second
	LeaderElectionRetryPeriod    = 2 * time.Second
	LogLevel                     = "info"
	LogFormat                    = "json"
	LeaderElectionLeaseName      = "mlb-controller-lease"
	LeaderElectionLeaseNamespace = "default"
	MetricsPort                  = 9090
	MetricsURI                   = "/metrics"
	KubernetesResyncPeriod       = 30 * time.Second
	RequestPoolSize              = 10
	RequestTimeOut               = 30 * time.Second
	FailTimeout                  = 60 * time.Second
)

// ApplyDefaultValues sets default values for the configuration.
func ApplyDefaultValues(cfg *Config) {
	// Initialize nil slices to empty slices
	if cfg.LoadBalancers.LoadBalancers == nil {
		cfg.LoadBalancers.LoadBalancers = []LoadBalancerConfig{}
	}

	// Set global defaults
	if cfg.GlobalUpstreamSyncPeriod == 0 {
		cfg.GlobalUpstreamSyncPeriod = GlobalUpstreamSyncPeriod
	}
	if cfg.LeaderElection.LeaseName == "" {
		cfg.LeaderElection.LeaseName = LeaderElectionLeaseName
	}
	if cfg.LeaderElection.LeaseNamespace == "" {
		cfg.LeaderElection.LeaseNamespace = LeaderElectionLeaseNamespace
	}
	if cfg.LeaderElection.LeaseDuration == 0 {
		cfg.LeaderElection.LeaseDuration = LeaderElectionLeaseDuration
	}
	if cfg.LeaderElection.RenewDeadline == 0 {
		cfg.LeaderElection.RenewDeadline = LeaderElectionRenewDeadline
	}
	if cfg.LeaderElection.RetryPeriod == 0 {
		cfg.LeaderElection.RetryPeriod = LeaderElectionRetryPeriod
	}
	if cfg.Log.Level == "" {
		cfg.Log.Level = LogLevel
	}
	if cfg.Log.Format == "" {
		cfg.Log.Format = LogFormat
	}
	if cfg.Metrics.Port == 0 {
		cfg.Metrics.Port = MetricsPort
	}
	if cfg.Metrics.URI == "" {
		cfg.Metrics.URI = MetricsURI
	}
	if cfg.Kubernetes.ResyncPeriod == 0 {
		cfg.Kubernetes.ResyncPeriod = KubernetesResyncPeriod
	}
	if cfg.RequestPoolSize == 0 {
		cfg.RequestPoolSize = RequestPoolSize
	}

	// Apply defaults for each load balancer
	for i, lb := range cfg.LoadBalancers.LoadBalancers {
		lbWithDefaults := lb.SetDefaults(cfg.GlobalUpstreamSyncPeriod, cfg.RequestPoolSize)
		switch concrete := lbWithDefaults.(type) {
		case NginxConfig:
			if !concrete.AllowEmptyUpstream && cfg.AllowEmptyUpstream {
				concrete.AllowEmptyUpstream = true
			}
			if concrete.DefaultBackend == nil && cfg.DefaultBackend != nil {
				concrete.DefaultBackend = cfg.DefaultBackend
			}
			cfg.LoadBalancers.LoadBalancers[i] = concrete
		case *NginxConfig:
			if concrete != nil {
				if !concrete.AllowEmptyUpstream && cfg.AllowEmptyUpstream {
					concrete.AllowEmptyUpstream = true
				}
				if concrete.DefaultBackend == nil && cfg.DefaultBackend != nil {
					concrete.DefaultBackend = cfg.DefaultBackend
				}
				cfg.LoadBalancers.LoadBalancers[i] = concrete
			}
		case EnvoyConfig:
			if !concrete.AllowEmptyUpstream && cfg.AllowEmptyUpstream {
				concrete.AllowEmptyUpstream = true
			}
			if concrete.DefaultBackend == nil && cfg.DefaultBackend != nil {
				concrete.DefaultBackend = cfg.DefaultBackend
			}
			cfg.LoadBalancers.LoadBalancers[i] = concrete
		case *EnvoyConfig:
			if concrete != nil {
				if !concrete.AllowEmptyUpstream && cfg.AllowEmptyUpstream {
					concrete.AllowEmptyUpstream = true
				}
				if concrete.DefaultBackend == nil && cfg.DefaultBackend != nil {
					concrete.DefaultBackend = cfg.DefaultBackend
				}
				cfg.LoadBalancers.LoadBalancers[i] = concrete
			}
		default:
			cfg.LoadBalancers.LoadBalancers[i] = lbWithDefaults
		}
	}
}
