package config

import (
	"fmt"
	domain "mlb-controller/internal/domain/config"

	"github.com/spf13/viper"
)

type ViperLoader struct {
	path string
}

func NewViperLoader(path string) *ViperLoader {
	return &ViperLoader{path: path}
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

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validate config: %w", err)
	}

	return cfg, nil
}
