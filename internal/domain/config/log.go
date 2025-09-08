package config

// LogConfig defines logger configuration.
type LogConfig struct {
	Enabled bool   `mapstructure:"enabled"`
	Level   string `mapstructure:"level"`  // debug, info, warn, error, fatal
	Format  string `mapstructure:"format"` // json, console
}
