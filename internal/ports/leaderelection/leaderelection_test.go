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
	cbStart := f.callbacks.OnStartedLeading
	f.mu.Unlock()

	if cbStart != nil {
		go cbStart(ctx)
	}

	// Simulate leadership until context is canceled
	<-ctx.Done()

	f.mu.Lock()
	f.isLeader = false
	f.onStopCalled = true
	cbStop := f.callbacks.OnStoppedLeading
	f.mu.Unlock()

	if cbStop != nil {
		cbStop()
	}

	return nil
}

func (f *fakeLeaderElection) IsLeader() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.isLeader
}

func (f *fakeLeaderElection) SetCallbacks(callbacks Callbacks) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.callbacks = callbacks
}

func (f *fakeLeaderElection) GetLeaderAddr() string {
	return "127.0.0.1:9090"
}

func TestLeaderElectionPort(t *testing.T) {
	t.Run("Run starts and stops leadership", func(t *testing.T) {
		var mu sync.Mutex
		var started, stopped bool

		fake := &fakeLeaderElection{
			callbacks: Callbacks{
				OnStartedLeading: func(ctx context.Context) {
					mu.Lock()
					started = true
					mu.Unlock()
				},
				OnStoppedLeading: func() {
					mu.Lock()
					stopped = true
					mu.Unlock()
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

		mu.Lock()
		isStarted := started
		mu.Unlock()
		assert.True(t, isStarted, "OnStartedLeading should be called")

		// Wait for context to cancel
		<-ctx.Done()
		time.Sleep(50 * time.Millisecond) // Allow time for cleanup
		assert.False(t, fake.IsLeader(), "should not be leader after context cancel")

		mu.Lock()
		isStopped := stopped
		mu.Unlock()
		assert.True(t, isStopped, "OnStoppedLeading should be called")
	})
}
