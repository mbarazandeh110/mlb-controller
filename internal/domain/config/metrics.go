package config

// MetricsConfig defines settings for Prometheus metrics.
type MetricsConfig struct {
	Enabled bool   `mapstructure:"enabled"`
	Port    int    `mapstructure:"port"`
	URI     string `mapstructure:"uri"`
}
