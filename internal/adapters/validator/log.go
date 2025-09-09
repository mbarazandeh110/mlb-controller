package validator

import (
	"fmt"
	domain "mlb-controller/internal/domain/config"
	"strings"
)

// LogValidator validates logging configuration.
type LogValidator struct{}

func (v *LogValidator) Validate(cfg *domain.Config) error {
	if !cfg.Log.Enabled {
		return nil
	}
	switch strings.ToLower(cfg.Log.Level) {
	case "debug", "info", "warn", "error", "fatal":
	default:
		return fmt.Errorf("log.level must be one of: debug, info, warn, error, fatal")
	}
	switch strings.ToLower(cfg.Log.Format) {
	case "json", "console":
	default:
		return fmt.Errorf("log.format must be one of: console, json")
	}
	return nil
}
