package leaderelection

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func (f *fakeLeaderElection) GetLeaderAddr(ctx context.Context) string {
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

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		runDone := make(chan struct{})
		go func() {
			err := fake.Run(ctx)
			assert.NoError(t, err)
			close(runDone)
		}()

		require.Eventually(t, func() bool {
			mu.Lock()
			defer mu.Unlock()
			return fake.IsLeader() && started
		}, 1*time.Second, 10*time.Millisecond, "should be leader and OnStartedLeading should be called")

		// Stop leadership
		cancel()
		<-runDone

		require.Eventually(t, func() bool {
			mu.Lock()
			defer mu.Unlock()
			return !fake.IsLeader() && stopped
		}, 1*time.Second, 10*time.Millisecond, "should not be leader and OnStoppedLeading should be called")
	})
}
