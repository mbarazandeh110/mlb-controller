// internal/controller/controller.go
package controller

import (
	"context"
	"fmt"
	"sync"
	"time"

	"mlb-controller/internal/domain/config"
	"mlb-controller/internal/domain/model"
	"mlb-controller/internal/ports/kubernetes"
	"mlb-controller/internal/ports/loadbalancer"
	"mlb-controller/internal/ports/logging"
	"mlb-controller/internal/ports/metrics"
	"mlb-controller/internal/util"
)

// ControllerImpl implements the controller.Controller interface.
type ControllerImpl struct {
	k8sClient       kubernetes.KubernetesPort
	logger          logging.Logger
	metrics         metrics.Metrics
	loadBalancers   map[string]loadbalancer.LoadBalancerAdapter // Key: loadbalancer name
	config          *config.Config
	state           model.ControllerState
	stateMutex      sync.RWMutex
	serviceEvents   chan kubernetes.ServiceEvent
	podEvents       chan kubernetes.PodEvent
	nodeEvents      chan kubernetes.NodeEvent
	syncTicker      *time.Ticker
	syncStop        chan struct{}
	syncUpstreamCtr metrics.Counter
}

// NewController creates a new ControllerImpl instance.
func NewController(
	k8sClient kubernetes.KubernetesPort,
	logger logging.Logger,
	metrics metrics.Metrics,
	lbAdapters map[string]loadbalancer.LoadBalancerAdapter,
	cfg *config.Config,
) *ControllerImpl {
	return &ControllerImpl{
		k8sClient:     k8sClient,
		logger:        logger,
		metrics:       metrics,
		loadBalancers: lbAdapters,
		config:        cfg,
		state: model.ControllerState{
			Services:  make(map[string]model.Service),
			Pods:      make(map[string]model.Pod),
			Nodes:     make(map[string]model.Node),
			Upstreams: make(map[string]model.Upstream),
		},
		serviceEvents:   make(chan kubernetes.ServiceEvent, 100), // Buffered to prevent missing events
		podEvents:       make(chan kubernetes.PodEvent, 100),
		nodeEvents:      make(chan kubernetes.NodeEvent, 100),
		syncStop:        make(chan struct{}),
		syncUpstreamCtr: metrics.NewCounter("sync_upstreams_total", "Total number of upstream sync operations", nil),
	}
}

// Start initializes the controller and begins monitoring Kubernetes resources.
func (c *ControllerImpl) Start(ctx context.Context) error {
	c.logger.Info("Starting controller")

	// Start watching Kubernetes resources
	go func() {
		if err := c.k8sClient.WatchServices(ctx, nil, c.serviceEvents); err != nil {
			c.logger.Error("Failed to watch services", logging.Field{Key: "error", Value: err})
			c.metrics.IncrementSyncErrors("watch_services")
		}
	}()
	go func() {
		if err := c.k8sClient.WatchPods(ctx, c.podEvents); err != nil {
			c.logger.Error("Failed to watch pods", logging.Field{Key: "error", Value: err})
			c.metrics.IncrementSyncErrors("watch_pods")
		}
	}()
	go func() {
		if err := c.k8sClient.WatchNodes(ctx, c.nodeEvents); err != nil {
			c.logger.Error("Failed to watch nodes", logging.Field{Key: "error", Value: err})
			c.metrics.IncrementSyncErrors("watch_nodes")
		}
	}()

	// Start periodic sync (requirement 7)
	c.syncTicker = time.NewTicker(c.config.GlobalUpstreamSyncPeriod)
	go c.runPeriodicSync(ctx)

	// Process events
	go c.processEvents(ctx)

	// Initial sync on start (requirement 11)
	if err := c.SyncUpstreams(ctx); err != nil {
		c.logger.Error("Initial upstream sync failed", logging.Field{Key: "error", Value: err})
		return err
	}

	<-ctx.Done()
	c.logger.Info("Controller stopped")
	c.stop()
	return ctx.Err()
}

// stop cleans up resources.
func (c *ControllerImpl) stop() {
	if c.syncTicker != nil {
		c.syncTicker.Stop()
	}
	close(c.syncStop)
}

// processEvents handles incoming Kubernetes events.
func (c *ControllerImpl) processEvents(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case svcEvent := <-c.serviceEvents:
			c.handleServiceEvent(svcEvent)
		case podEvent := <-c.podEvents:
			c.handlePodEvent(podEvent)
		case nodeEvent := <-c.nodeEvents:
			c.handleNodeEvent(nodeEvent)
		}
	}
}

// handleServiceEvent processes Kubernetes Service events.
func (c *ControllerImpl) handleServiceEvent(event kubernetes.ServiceEvent) {
	key := fmt.Sprintf("%s/%s", event.Service.Namespace, event.Service.Name)
	c.stateMutex.Lock()
	defer c.stateMutex.Unlock()

	switch event.Type {
	case kubernetes.EventTypeAdd, kubernetes.EventTypeUpdate:
		c.state.Services[key] = event.Service
		c.logger.Info("Service updated in state", logging.Field{Key: "service", Value: key})
	case kubernetes.EventTypeDelete:
		delete(c.state.Services, key)
		c.logger.Info("Service removed from state", logging.Field{Key: "service", Value: key})
	}

	// Trigger upstream sync after service change (requirement 4)
	if err := c.syncUpstreamsForService(context.Background(), event.Service); err != nil {
		c.logger.Error("Failed to sync upstreams for service", logging.Field{Key: "service", Value: key}, logging.Field{Key: "error", Value: err})
	}
}

// handlePodEvent processes Kubernetes Pod events (requirement 3).
func (c *ControllerImpl) handlePodEvent(event kubernetes.PodEvent) {
	key := fmt.Sprintf("%s/%s", event.Pod.Namespace, event.Pod.Name)
	c.stateMutex.Lock()
	defer c.stateMutex.Unlock()

	switch event.Type {
	case kubernetes.EventTypeAdd, kubernetes.EventTypeUpdate:
		c.state.Pods[key] = event.Pod
		c.logger.Info("Pod updated in state", logging.Field{Key: "pod", Value: key}, logging.Field{Key: "ready", Value: event.Pod.Ready})
	case kubernetes.EventTypeDelete:
		delete(c.state.Pods, key)
		c.logger.Info("Pod removed from state", logging.Field{Key: "pod", Value: key})
	}

	// Trigger upstream sync for affected services (requirement 4)
	c.syncUpstreamsForPod(event.Pod)
}

// handleNodeEvent processes Kubernetes Node events.
func (c *ControllerImpl) handleNodeEvent(event kubernetes.NodeEvent) {
	c.stateMutex.Lock()
	defer c.stateMutex.Unlock()

	switch event.Type {
	case kubernetes.EventTypeAdd, kubernetes.EventTypeUpdate:
		c.state.Nodes[event.Node.Name] = event.Node
		c.logger.Info("Node updated in state", logging.Field{Key: "node", Value: event.Node.Name})
	case kubernetes.EventTypeDelete:
		delete(c.state.Nodes, event.Node.Name)
		c.logger.Info("Node removed from state", logging.Field{Key: "node", Value: event.Node.Name})
	}

	// Trigger upstream sync for affected pods (requirement 2)
	c.syncUpstreamsForNode(event.Node)
}

// syncUpstreamsForService syncs upstreams for a specific service.
func (c *ControllerImpl) syncUpstreamsForService(ctx context.Context, svc model.Service) error {
	startTime := time.Now()
	upstreamName := svc.Labels["mlb-upstream-name"]

	if svc.Labels["mlb-loadbalancer-name"] == "" || upstreamName == "" {
		return nil // Not an MLB service
	}

	pods, err := c.k8sClient.GetPodsForSelector(ctx, svc.Namespace, svc.Selector)
	if err != nil {
		c.metrics.IncrementSyncErrors(upstreamName)
		return fmt.Errorf("failed to get pods for service %s/%s: %w", svc.Namespace, svc.Name, err)
	}

	upstream := c.buildUpstream(svc, pods)
	if upstream == nil {
		c.metrics.UpdateBackendsTotal(upstreamName, 0)
		return nil // No valid upstream (e.g., no ready pods)
	}

	// Update backends total metric
	c.metrics.UpdateBackendsTotal(upstreamName, float64(len(upstream.Backends)))

	key := fmt.Sprintf("%s/%s", svc.Labels["mlb-loadbalancer-name"], svc.Labels["mlb-upstream-name"])
	c.stateMutex.Lock()
	c.state.Upstreams[key] = *upstream
	c.stateMutex.Unlock()

	if err := c.syncLoadBalancerUpstream(ctx, upstream); err != nil {
		c.metrics.IncrementSyncErrors(upstreamName)
		return err
	}

	// Observe sync duration
	duration := time.Since(startTime).Seconds()
	c.metrics.ObserveSyncDuration(upstreamName, duration)

	return nil
}

// syncUpstreamsForPod syncs upstreams for services affected by a pod change.
func (c *ControllerImpl) syncUpstreamsForPod(pod model.Pod) {
	c.stateMutex.RLock()
	defer c.stateMutex.RUnlock()

	for _, svc := range c.state.Services {
		if c.matchLabels(pod.Labels, svc.Selector) {
			if err := c.syncUpstreamsForService(context.Background(), svc); err != nil {
				c.logger.Error("Failed to sync upstreams for pod change",
					logging.Field{Key: "pod", Value: fmt.Sprintf("%s/%s", pod.Namespace, pod.Name)},
					logging.Field{Key: "service", Value: fmt.Sprintf("%s/%s", svc.Namespace, svc.Name)},
					logging.Field{Key: "error", Value: err})
			}
		}
	}
}

// syncUpstreamsForNode syncs upstreams for pods running on a specific node.
func (c *ControllerImpl) syncUpstreamsForNode(node model.Node) {
	c.stateMutex.RLock()
	defer c.stateMutex.RUnlock()

	for _, pod := range c.state.Pods {
		if pod.NodeName == node.Name {
			c.syncUpstreamsForPod(pod)
		}
	}
}

// runPeriodicSync performs periodic upstream synchronization (requirement 7).
func (c *ControllerImpl) runPeriodicSync(ctx context.Context) {
	for {
		select {
		case <-c.syncTicker.C:
			if err := c.SyncUpstreams(ctx); err != nil {
				c.logger.Error("Periodic upstream sync failed", logging.Field{Key: "error", Value: err})
			}
		case <-c.syncStop:
			return
		case <-ctx.Done():
			return
		}
	}
}

// SyncUpstreams synchronizes all upstreams for monitored services (requirements 7, 11).
func (c *ControllerImpl) SyncUpstreams(ctx context.Context) error {
	startTime := time.Now()
	c.stateMutex.RLock()
	services := make([]model.Service, 0, len(c.state.Services))
	for _, svc := range c.state.Services {
		services = append(services, svc)
	}
	c.stateMutex.RUnlock()

	var errs []error
	for _, svc := range services {
		if err := c.syncUpstreamsForService(ctx, svc); err != nil {
			errs = append(errs, err)
		}
	}

	c.syncUpstreamCtr.Inc()
	// Observe sync duration for all upstreams
	duration := time.Since(startTime).Seconds()
	c.metrics.ObserveSyncDuration("all_upstreams", duration)

	if len(errs) > 0 {
		c.metrics.IncrementSyncErrors("all_upstreams")
		return fmt.Errorf("errors during upstream sync: %v", errs)
	}
	return nil
}

// buildUpstream constructs an Upstream object from a service and its pods (requirements 2, 5, 14).
func (c *ControllerImpl) buildUpstream(svc model.Service, pods []model.Pod) *model.Upstream {
	lbName := svc.Labels["mlb-loadbalancer-name"]
	upstreamName := svc.Labels["mlb-upstream-name"]
	if lbName == "" || upstreamName == "" || svc.NodePort == 0 {
		return nil
	}

	var backends []model.Backend
	for _, pod := range pods {
		if !pod.Ready || pod.NodeName == "" {
			continue
		}

		node, err := c.k8sClient.GetNodeByName(context.Background(), pod.NodeName)
		if err != nil {
			c.logger.Error("Failed to get node for pod",
				logging.Field{Key: "pod", Value: fmt.Sprintf("%s/%s", pod.Namespace, pod.Name)},
				logging.Field{Key: "node", Value: pod.NodeName},
				logging.Field{Key: "error", Value: err})
			c.metrics.IncrementSyncErrors(upstreamName)
			continue
		}

		nodeIP := node.InternalIP
		if nodeIP == "" {
			nodeIP = node.ExternalIP
		}
		if nodeIP == "" {
			c.logger.Warn("No valid IP for node", logging.Field{Key: "node", Value: node.Name})
			continue
		}

		// Apply IP replacement if enabled (requirement 14)
		lbConfig, exists := c.getLoadBalancerConfig(lbName)
		if exists && lbConfig.GetIPReplacement() {
			nodeIP = util.ApplyIPReplacement(nodeIP, lbConfig, c.config.GlobalIPReplacementList)
		}

		backends = append(backends, model.Backend{
			IP:     nodeIP,
			Port:   svc.NodePort,
			Weight: 1, // Requirement 5
		})
	}

	if len(backends) == 0 {
		return nil
	}

	lbConfig, _ := c.getLoadBalancerConfig(lbName) // Get config for upstream
	return &model.Upstream{
		Name:         upstreamName,
		LoadBalancer: lbName,
		Backends:     backends,
		Type:         c.getLoadBalancerType(lbName),
		Config:       lbConfig, // Use the config directly
	}
}

// syncLoadBalancerUpstream calls the load balancer adapter to sync the upstream.
func (c *ControllerImpl) syncLoadBalancerUpstream(ctx context.Context, upstream *model.Upstream) error {
	if upstream == nil {
		return nil
	}

	adapter, exists := c.loadBalancers[upstream.LoadBalancer]
	if !exists {
		c.metrics.IncrementSyncErrors(upstream.Name)
		return fmt.Errorf("load balancer adapter not found: %s", upstream.LoadBalancer)
	}

	if err := adapter.SyncUpstream(ctx, *upstream); err != nil {
		c.metrics.IncrementSyncErrors(upstream.Name)
		return fmt.Errorf("failed to sync upstream %s/%s: %w", upstream.LoadBalancer, upstream.Name, err)
	}

	c.logger.Info("Upstream synced successfully",
		logging.Field{Key: "loadbalancer", Value: upstream.LoadBalancer},
		logging.Field{Key: "upstream", Value: upstream.Name},
		logging.Field{Key: "backends", Value: len(upstream.Backends)})
	return nil
}

// getLoadBalancerConfig retrieves the load balancer config by name.
func (c *ControllerImpl) getLoadBalancerConfig(lbName string) (config.LoadBalancerConfig, bool) {
	for _, lb := range c.config.LoadBalancers.LoadBalancers {
		if lb.GetName() == lbName {
			return lb, true
		}
	}
	return nil, false
}

// getLoadBalancerType retrieves the load balancer type by name.
func (c *ControllerImpl) getLoadBalancerType(lbName string) string {
	for _, lb := range c.config.LoadBalancers.LoadBalancers {
		if lb.GetName() == lbName {
			return lb.GetType()
		}
	}
	return ""
}

// matchLabels checks if pod labels match service selector.
func (c *ControllerImpl) matchLabels(podLabels, svcSelector map[string]string) bool {
	for k, v := range svcSelector {
		if podLabels[k] != v {
			return false
		}
	}
	return true
}

// GetState returns the current state of the controller.
func (c *ControllerImpl) GetState() model.ControllerState {
	c.stateMutex.RLock()
	defer c.stateMutex.RUnlock()
	return c.state
}
