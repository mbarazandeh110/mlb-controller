package config

import (
	"context"

	domain "mlb-controller/internal/domain/config"
)

type Loader interface {
	Load(ctx context.Context) (*domain.Config, error)
}
