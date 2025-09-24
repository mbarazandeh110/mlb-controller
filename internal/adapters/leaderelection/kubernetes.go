package leaderelection

import (
	"context"
	"fmt"
	"os"

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
		k.isLeader = true
		if k.callbacks.OnStartedLeading != nil {
			k.callbacks.OnStartedLeading(ctx)
		}
		<-ctx.Done()
		k.isLeader = false
		if k.callbacks.OnStoppedLeading != nil {
			k.callbacks.OnStoppedLeading()
		}
		return nil
	}

	// Get pod identity (use hostname as ID)
	id, err := os.Hostname()
	if err != nil {
		return fmt.Errorf("failed to get hostname: %w", err)
	}

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
		Lock:          lock,
		LeaseDuration: k.config.LeaseDuration,
		RenewDeadline: k.config.RenewDeadline,
		RetryPeriod:   k.config.RetryPeriod,
		Callbacks: k8sleaderelection.LeaderCallbacks{
			OnStartedLeading: func(ctx context.Context) {
				k.logger.Info("Became leader", logging_ports.Field{Key: "id", Value: id})
				k.isLeader = true
				if k.callbacks.OnStartedLeading != nil {
					k.callbacks.OnStartedLeading(ctx)
				}
			},
			OnStoppedLeading: func() {
				k.logger.Info("Stopped being leader", logging_ports.Field{Key: "id", Value: id})
				k.isLeader = false
				if k.callbacks.OnStoppedLeading != nil {
					k.callbacks.OnStoppedLeading()
				}
			},
		},
	}

	// Start leader election
	leaderElector, err := k8sleaderelection.NewLeaderElector(leConfig)
	if err != nil {
		return fmt.Errorf("failed to create leader elector: %w", err)
	}

	go leaderElector.Run(ctx)
	<-ctx.Done()
	return nil
}

// IsLeader returns true if the current instance is the leader.
func (k *KubernetesLeaderElection) IsLeader() bool {
	return k.isLeader
}

// SetCallbacks sets the callbacks for leader election events.
func (k *KubernetesLeaderElection) SetCallbacks(callbacks leaderelection_ports.Callbacks) {
	k.callbacks = callbacks
}

// GetLeaderAddr returns the address of the current leader by querying the Lease object.
func (k *KubernetesLeaderElection) GetLeaderAddr() string {
	if !k.config.Enabled {
		// When leader election is disabled, use local metrics address
		id, err := os.Hostname()
		if err != nil {
			k.logger.Error("Failed to get hostname for leader address", logging_ports.Field{Key: "error", Value: err})
			return ""
		}
		return id
	}

	if k.isLeader {
		// If this instance is the leader, return its own address
		id, err := os.Hostname()
		if err != nil {
			k.logger.Error("Failed to get hostname for leader address", logging_ports.Field{Key: "error", Value: err})
			return ""
		}
		return id
	}

	// Query the Lease object to get the leader's identity
	lease, err := k.client.CoordinationV1().Leases(k.config.LeaseNamespace).Get(context.Background(), k.config.LeaseName, metav1.GetOptions{})
	if err != nil {
		k.logger.Error("Failed to get lease for leader address", logging_ports.Field{Key: "error", Value: err})
		return ""
	}

	if lease.Spec.HolderIdentity == nil {
		k.logger.Warn("No leader found in lease")
		return ""
	}

	// Use holderIdentity (Pod name) as the leader address with MetricsPort
	return *lease.Spec.HolderIdentity
}
