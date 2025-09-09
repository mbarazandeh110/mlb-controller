package config

import (
	"fmt"
	validator "mlb-controller/internal/adapters/validator"
	domain "mlb-controller/internal/domain/config"
	config_ports "mlb-controller/internal/ports/config"

	"github.com/spf13/viper"
)

type ViperLoader struct {
	path      string
	validator config_ports.Validator
}

func NewViperLoader(path string) *ViperLoader {
	return &ViperLoader{
		path:      path,
		validator: validator.NewCompositeValidator(),
	}
}

func (l *ViperLoader) Load() (*domain.Config, error) {
	v := viper.New()
	v.SetConfigFile(l.path)

	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	cfg := &domain.Config{}
	if err := v.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}

	// Apply default values before validation
	domain.ApplyDefaultValues(cfg)

	if err := l.validator.Validate(cfg); err != nil {
		return nil, fmt.Errorf("validate config: %w", err)
	}

	return cfg, nil
}
