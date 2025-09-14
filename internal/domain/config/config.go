package config

import (
	"time"
)

// Config is the root configuration structure.
type Config struct {
	GlobalUpstreamSyncPeriod time.Duration           `mapstructure:"global_upstream_sync_period"`
	LeaderElection           LeaderElectionConfig    `mapstructure:"leader_election"`
	Log                      LogConfig               `mapstructure:"log"`
	Metrics                  MetricsConfig           `mapstructure:"metrics"`
	Kubernetes               KubernetesConfig        `mapstructure:"kubernetes"`
	GlobalIPReplacementList  GlobalIPReplacementList `mapstructure:"global_ip_replacement_list"`
	LoadBalancers            LoadBalancersConfig     `mapstructure:"loadbalancers"`
	RequestPoolSize          int                     `mapstructure:"request_pool_size"` // Maximum concurrent requests to load balancers
}
