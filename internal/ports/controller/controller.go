// internal/ports/controller/controller.go
package controller

import (
	"context"
	"mlb-controller/internal/domain/model"
)

// Controller defines the interface for the main controller logic.
type Controller interface {
	// Start initializes the controller and begins monitoring Kubernetes resources.
	Start(ctx context.Context) error
	// SyncUpstreams synchronizes the upstreams for all monitored services.
	SyncUpstreams(ctx context.Context) error
	// GetState returns the current state of the controller.
	GetState() model.ControllerState
}
