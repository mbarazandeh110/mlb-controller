// internal/di/wire.go
package di

import (
	"fmt"

	adapters_certificate "mlb-controller/internal/adapters/certificate"
	adapters_config "mlb-controller/internal/adapters/config"
	adapters_kubernetes "mlb-controller/internal/adapters/kubernetes"
	adapters_leaderelection "mlb-controller/internal/adapters/leaderelection"
	adapters_loadbalancer "mlb-controller/internal/adapters/loadbalancer"
	adapters_logging "mlb-controller/internal/adapters/logging"
	adapters_metrics "mlb-controller/internal/adapters/metrics"
	"mlb-controller/internal/application"
	mlb_controller "mlb-controller/internal/controller"
	configports "mlb-controller/internal/ports/config"
	ports_controller "mlb-controller/internal/ports/controller"
	ports_kubernetes "mlb-controller/internal/ports/kubernetes"
	"mlb-controller/internal/ports/leaderelection"
	"mlb-controller/internal/ports/loadbalancer"
	"mlb-controller/internal/ports/logging"
	ports_metrics "mlb-controller/internal/ports/metrics"
)

// Container holds dependencies for the application.
type Container struct {
	Logger         logging.Logger
	Loader         configports.Loader
	Metrics        ports_metrics.Metrics
	LeaderElection leaderelection.LeaderElectionPort
	K8sClient      ports_kubernetes.KubernetesPort
	Controller     ports_controller.Controller
	App            *application.App
}

// NewContainer creates and wires dependencies.
func NewContainer(configPath string) (*Container, error) {
	// Initialize bootstrap logger
	bootstrapLogger, err := adapters_logging.New(adapters_logging.LogConfig{Level: "info", Format: "json"})
	if err != nil {
		return nil, fmt.Errorf("failed to initialize bootstrap logger: %w", err)
	}

	// Create a new certificate loader instance
	certLoader := adapters_certificate.NewFileLoader()

	// Load config to determine production logger, metrics, and leader election settings
	loader := adapters_config.NewViperLoader(configPath, certLoader)
	cfg, err := loader.Load()
	if err != nil {
		bootstrapLogger.Fatal("Failed to load config", logging.Field{Key: "error", Value: err})
	}

	// Initialize production logger
	prodLoggerCfg := adapters_logging.LogConfig{Level: cfg.Log.Level, Format: cfg.Log.Format}
	prodLogger, err := adapters_logging.New(prodLoggerCfg)
	if err != nil {
		bootstrapLogger.Error("Failed to initialize production logger", logging.Field{Key: "error", Value: err})
		prodLogger = bootstrapLogger
	}

	// Initialize Kubernetes client
	k8sClient, err := adapters_kubernetes.NewKubernetesAdapter(cfg.Kubernetes, prodLogger)
	if err != nil {
		prodLogger.Error("Failed to initialize Kubernetes client", logging.Field{Key: "error", Value: err})
		return nil, fmt.Errorf("failed to initialize Kubernetes client: %w", err)
	}

	// Initialize leader election adapter
	leaderElectionAdapter, err := adapters_leaderelection.NewKubernetesLeaderElection(cfg.LeaderElection, cfg.Kubernetes, prodLogger)
	if err != nil {
		prodLogger.Error("Failed to initialize leader election adapter", logging.Field{Key: "error", Value: err})
		return nil, fmt.Errorf("failed to initialize leader election: %w", err)
	}

	// Initialize metrics adapter
	metricsAdapter := adapters_metrics.NewPrometheusAdapter(prodLogger, leaderElectionAdapter, cfg.Metrics)

	// Initialize load balancer adapters using Factory
	factory := adapters_loadbalancer.NewLoadBalancerFactory()
	lbAdapters := make(map[string]loadbalancer.LoadBalancerAdapter)
	for _, lb := range cfg.LoadBalancers.LoadBalancers {
		adapter, err := factory.CreateLoadBalancerAdapter(lb, prodLogger)
		if err != nil {
			prodLogger.Error("Failed to create load balancer adapter",
				logging.Field{Key: "name", Value: lb.GetName()},
				logging.Field{Key: "type", Value: lb.GetType()},
				logging.Field{Key: "error", Value: err})
			return nil, fmt.Errorf("failed to create load balancer adapter for %s: %w", lb.GetName(), err)
		}
		lbAdapters[lb.GetName()] = adapter
	}

	// Initialize controller
	ctrl := mlb_controller.NewController(k8sClient, prodLogger, metricsAdapter, lbAdapters, cfg)

	// Create App
	app := application.NewApp(prodLogger, loader, metricsAdapter, leaderElectionAdapter, ctrl)

	return &Container{
		Logger:         prodLogger,
		Loader:         loader,
		Metrics:        metricsAdapter,
		LeaderElection: leaderElectionAdapter,
		K8sClient:      k8sClient,
		Controller:     ctrl,
		App:            app,
	}, nil
}
