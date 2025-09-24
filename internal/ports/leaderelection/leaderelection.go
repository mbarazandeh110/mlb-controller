// internal/ports/leaderelection/leaderelection.go
package leaderelection

import "context"

// LeaderElectionPort defines the interface for leader election mechanisms.
type LeaderElectionPort interface {
	// Run starts the leader election process and blocks until the context is canceled.
	// Callbacks (OnStartedLeading, OnStoppedLeading) are invoked on leadership changes.
	Run(ctx context.Context) error
	// IsLeader checks if the current instance is the leader.
	IsLeader() bool
	// SetCallbacks sets the callbacks for leader election events.
	SetCallbacks(callbacks Callbacks)
	// GetLeaderAddr returns the address of the current leader (e.g., Pod IP or service endpoint).
	GetLeaderAddr() string
}

// Callbacks defines the callbacks for leader election events.
type Callbacks struct {
	// OnStartedLeading is called when this instance becomes the leader.
	OnStartedLeading func(ctx context.Context)

	// OnStoppedLeading is called when this instance loses leadership.
	OnStoppedLeading func()
}
