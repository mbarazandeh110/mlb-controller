package config

import domain "mlb-controller/internal/domain/config"

type Loader interface {
	Load() (*domain.Config, error)
}
