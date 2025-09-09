package config

import (
	domain "mlb-controller/internal/domain/config"
)

// Validator defines the interface for configuration validation.
type Validator interface {
	Validate(cfg *domain.Config) error
}
