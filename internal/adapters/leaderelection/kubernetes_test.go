package leaderelection

import (
	"context"
	"sync"
	"testing"
	"time"

	domain "mlb-controller/internal/domain/config"
	leaderelection_ports "mlb-controller/internal/ports/leaderelection"
	logging_ports "mlb-controller/internal/ports/logging"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
	coordinationv1 "k8s.io/api/coordination/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

type fakeLogger struct {
	logger *zap.Logger
}

func (f *fakeLogger) Debug(msg string, fields ...logging_ports.Field) {}
func (f *fakeLogger) Info(msg string, fields ...logging_ports.Field)  {}
func (f *fakeLogger) Warn(msg string, fields ...logging_ports.Field)  {}
func (f *fakeLogger) Error(msg string, fields ...logging_ports.Field) {}
func (f *fakeLogger) Fatal(msg string, fields ...logging_ports.Field) {}
func (f *fakeLogger) With(fields ...logging_ports.Field) logging_ports.Logger {
	return f
}
func (f *fakeLogger) Sync() error { return nil }

func TestKubernetesLeaderElection(t *testing.T) {
	fakeClient := fake.NewSimpleClientset()
	zapLogger := zaptest.NewLogger(t)
	logger := &fakeLogger{logger: zapLogger}

	leaderCfg := domain.LeaderElectionConfig{
		Enabled:        true,
		LeaseName:      "mlb-controller-leader",
		LeaseNamespace: "default",
		LeaseDuration:  15 * time.Second,
		RenewDeadline:  10 * time.Second,
		RetryPeriod:    2 * time.Second,
	}

	t.Run("Disabled leader election runs as leader", func(t *testing.T) {
		leaderCfg.Enabled = false
		le := &KubernetesLeaderElection{
			client:   fakeClient,
			config:   leaderCfg,
			logger:   logger,
			isLeader: false,
		}

		var started, stopped bool
		var mu sync.Mutex
		le.SetCallbacks(leaderelection_ports.Callbacks{
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
		})

		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		go func() {
			err := le.Run(ctx)
			assert.NoError(t, err)
		}()

		require.Eventually(t, func() bool {
			mu.Lock()
			defer mu.Unlock()
			return le.IsLeader() && started && !stopped
		}, 2*time.Second, 10*time.Millisecond, "should be leader when election is disabled and OnStartedLeading called")

		<-ctx.Done()
		require.Eventually(t, func() bool {
			mu.Lock()
			defer mu.Unlock()
			return !le.IsLeader() && stopped
		}, 2*time.Second, 10*time.Millisecond, "should not be leader after context cancel and OnStoppedLeading called")
	})

	t.Run("Enabled leader election with fake client", func(t *testing.T) {
		leaderCfg.Enabled = true
		le := &KubernetesLeaderElection{
			client:   fakeClient,
			config:   leaderCfg,
			logger:   logger,
			isLeader: false,
		}

		var started, stopped bool
		var mu sync.Mutex
		le.SetCallbacks(leaderelection_ports.Callbacks{
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
		})

		lease := &coordinationv1.Lease{
			ObjectMeta: metav1.ObjectMeta{
				Name:      leaderCfg.LeaseName,
				Namespace: leaderCfg.LeaseNamespace,
			},
		}
		_, err := fakeClient.CoordinationV1().Leases(leaderCfg.LeaseNamespace).Create(context.Background(), lease, metav1.CreateOptions{})
		assert.NoError(t, err)

		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		go func() {
			err := le.Run(ctx)
			assert.NoError(t, err)
		}()

		require.Eventually(t, func() bool {
			mu.Lock()
			defer mu.Unlock()
			return started && !stopped
		}, 2*time.Second, 10*time.Millisecond, "OnStartedLeading should be called")

		<-ctx.Done()
		require.Eventually(t, func() bool {
			mu.Lock()
			defer mu.Unlock()
			return !le.IsLeader() && stopped
		}, 2*time.Second, 10*time.Millisecond, "should not be leader after context cancel and OnStoppedLeading called")
	})
}
