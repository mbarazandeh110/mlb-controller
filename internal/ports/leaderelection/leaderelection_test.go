package leaderelection

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// fakeLeaderElection is a fake implementation of LeaderElectionPort for testing.
type fakeLeaderElection struct {
	isLeader      bool
	onStartCalled bool
	onStopCalled  bool
	callbacks     Callbacks
	mu            sync.Mutex
}

func (f *fakeLeaderElection) Run(ctx context.Context) error {
	f.mu.Lock()
	f.isLeader = true
	f.onStartCalled = true
	if f.callbacks.OnStartedLeading != nil {
		go f.callbacks.OnStartedLeading(ctx) // Run in goroutine to avoid blocking
	}
	f.mu.Unlock()

	// Simulate leadership until context is canceled
	<-ctx.Done()

	f.mu.Lock()
	f.isLeader = false
	f.onStopCalled = true
	if f.callbacks.OnStoppedLeading != nil {
		f.callbacks.OnStoppedLeading()
	}
	f.mu.Unlock()

	return nil
}

func (f *fakeLeaderElection) IsLeader() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.isLeader
}

func TestLeaderElectionPort(t *testing.T) {
	t.Run("Run starts and stops leadership", func(t *testing.T) {
		var started, stopped bool
		fake := &fakeLeaderElection{
			callbacks: Callbacks{
				OnStartedLeading: func(ctx context.Context) {
					started = true
				},
				OnStoppedLeading: func() {
					stopped = true
				},
			},
		}

		// Create context with timeout
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		// Run leader election in a goroutine
		go func() {
			err := fake.Run(ctx)
			assert.NoError(t, err)
		}()

		// Wait briefly to ensure leadership starts
		time.Sleep(50 * time.Millisecond)
		assert.True(t, fake.IsLeader(), "should be leader")
		assert.True(t, started, "OnStartedLeading should be called")

		// Wait for context to cancel
		<-ctx.Done()
		time.Sleep(50 * time.Millisecond) // Allow time for cleanup
		assert.False(t, fake.IsLeader(), "should not be leader after context cancel")
		assert.True(t, stopped, "OnStoppedLeading should be called")
	})
}
