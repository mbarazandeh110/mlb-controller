package leaderelection

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	domain "mlb-controller/internal/domain/config"
	leaderelection_ports "mlb-controller/internal/ports/leaderelection"
	logging_ports "mlb-controller/internal/ports/logging"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	k8sleaderelection "k8s.io/client-go/tools/leaderelection"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
)

type KubernetesLeaderElection struct {
	client    kubernetes.Interface
	config    domain.LeaderElectionConfig
	logger    logging_ports.Logger
	isLeader  bool
	callbacks leaderelection_ports.Callbacks
	mu        sync.RWMutex
}

func NewKubernetesLeaderElection(leaderCfg domain.LeaderElectionConfig, k8sCfg domain.KubernetesConfig, logger logging_ports.Logger) (*KubernetesLeaderElection, error) {
	if !leaderCfg.Enabled {
		return &KubernetesLeaderElection{
			client:    nil,
			config:    leaderCfg,
			logger:    logger,
			isLeader:  false,
			callbacks: leaderelection_ports.Callbacks{},
		}, nil
	}
	config, err := clientcmd.BuildConfigFromFlags("", k8sCfg.KubernetesConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to build kubernetes config: %w", err)
	}
	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	return &KubernetesLeaderElection{
		client:    client,
		config:    leaderCfg,
		logger:    logger,
		isLeader:  false,
		callbacks: leaderelection_ports.Callbacks{},
	}, nil
}

func (k *KubernetesLeaderElection) Run(ctx context.Context) error {
	if !k.config.Enabled {
		k.logger.Info("Leader election is disabled, running as leader")
		k.mu.Lock()
		k.isLeader = true
		cbStart := k.callbacks.OnStartedLeading
		k.mu.Unlock()

		if cbStart != nil {
			cbStart(ctx)
		}
		<-ctx.Done()

		k.mu.Lock()
		k.isLeader = false
		cbStop := k.callbacks.OnStoppedLeading
		k.mu.Unlock()

		if cbStop != nil {
			cbStop()
		}
		return nil
	}

	// Get pod identity (use hostname + timestamp as unique ID)
	host, err := os.Hostname()
	if err != nil {
		host = "mlb-controller"
	}
	id := fmt.Sprintf("%s_%d", host, time.Now().UnixNano())

	// Create resource lock
	lock := &resourcelock.LeaseLock{
		LeaseMeta: metav1.ObjectMeta{
			Name:      k.config.LeaseName,
			Namespace: k.config.LeaseNamespace,
		},
		Client: k.client.CoordinationV1(),
		LockConfig: resourcelock.ResourceLockConfig{
			Identity: id,
		},
	}

	// Set up leader election config
	leConfig := k8sleaderelection.LeaderElectionConfig{
		Lock:            lock,
		LeaseDuration:   k.config.LeaseDuration,
		RenewDeadline:   k.config.RenewDeadline,
		RetryPeriod:     k.config.RetryPeriod,
		ReleaseOnCancel: true,
		Callbacks: k8sleaderelection.LeaderCallbacks{
			OnStartedLeading: func(ctx context.Context) {
				k.logger.Info("Became leader", logging_ports.Field{Key: "id", Value: id})
				k.mu.Lock()
				k.isLeader = true
				cb := k.callbacks.OnStartedLeading
				k.mu.Unlock()

				if cb != nil {
					cb(ctx)
				}
			},
			OnStoppedLeading: func() {
				k.logger.Info("Stopped being leader", logging_ports.Field{Key: "id", Value: id})
				k.mu.Lock()
				k.isLeader = false
				cb := k.callbacks.OnStoppedLeading
				k.mu.Unlock()

				if cb != nil {
					cb()
				}
			},
		},
	}

	// Start leader elector
	leaderElector, err := k8sleaderelection.NewLeaderElector(leConfig)
	if err != nil {
		return fmt.Errorf("failed to create leader elector: %w", err)
	}

	leaderElector.Run(ctx)
	if ctx.Err() != nil {
		return nil
	}
	return fmt.Errorf("leader election lease lost")
}

// IsLeader returns true if the current instance is the leader.
func (k *KubernetesLeaderElection) IsLeader() bool {
	k.mu.RLock()
	defer k.mu.RUnlock()
	return k.isLeader
}

// SetCallbacks sets the callbacks for leader election events.
func (k *KubernetesLeaderElection) SetCallbacks(callbacks leaderelection_ports.Callbacks) {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.callbacks = callbacks
}

// GetLeaderAddr returns the address of the current leader by querying the Lease object.
func (k *KubernetesLeaderElection) GetLeaderAddr(ctx context.Context) string {
	if !k.config.Enabled {
		id, err := os.Hostname()
		if err != nil {
			k.logger.Error("Failed to get hostname for leader address", logging_ports.Field{Key: "error", Value: err})
			return ""
		}
		return id
	}

	if k.IsLeader() {
		id, err := os.Hostname()
		if err != nil {
			k.logger.Error("Failed to get hostname for leader address", logging_ports.Field{Key: "error", Value: err})
			return ""
		}
		return id
	}

	lease, err := k.client.CoordinationV1().Leases(k.config.LeaseNamespace).Get(ctx, k.config.LeaseName, metav1.GetOptions{})
	if err != nil {
		k.logger.Error("Failed to get lease for leader address", logging_ports.Field{Key: "error", Value: err})
		return ""
	}

	if lease.Spec.HolderIdentity == nil {
		k.logger.Warn("No leader found in lease")
		return ""
	}

	return *lease.Spec.HolderIdentity
}
