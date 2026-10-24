// internal/controller/controller.go
package controller

import (
	"context"
	"fmt"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/workqueue"

	"mlb-controller/internal/domain/config"
	"mlb-controller/internal/domain/model"
	"mlb-controller/internal/ports/kubernetes"
	"mlb-controller/internal/ports/loadbalancer"
	"mlb-controller/internal/ports/logging"
	"mlb-controller/internal/ports/metrics"
	"mlb-controller/internal/util"
)

type ControllerImpl struct {
	k8sClient         kubernetes.KubernetesPort
	logger            logging.Logger
	metrics           metrics.Metrics
	loadBalancers     map[string]loadbalancer.LoadBalancerAdapter
	config            *config.Config
	state             model.ControllerState
	lastKnownServices map[string]model.Service
	stateMutex        sync.RWMutex
	serviceEvents     chan kubernetes.ServiceEvent
	podEvents         chan kubernetes.PodEvent
	nodeEvents        chan kubernetes.NodeEvent
	syncTicker        *time.Ticker
	syncStop          chan struct{}
	syncUpstreamCtr   metrics.Counter
	queue             workqueue.RateLimitingInterface
}

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
		lastKnownServices: make(map[string]model.Service),
		serviceEvents:     make(chan kubernetes.ServiceEvent, 500),
		podEvents:         make(chan kubernetes.PodEvent, 2000),
		nodeEvents:        make(chan kubernetes.NodeEvent, 500),
		syncStop:          make(chan struct{}),
		syncUpstreamCtr:   metrics.NewCounter("sync_upstreams_total", "Total number of upstream sync operations", nil),
		queue:             workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "mlb_sync_queue"),
	}
}

func (c *ControllerImpl) Start(ctx context.Context) error {
	c.logger.Info("Starting controller with Two-Way Reconciliation Engine")

	go func() {
		if err := c.k8sClient.WatchServices(ctx, nil, c.serviceEvents); err != nil {
			c.logger.Error("Failed to watch services", logging.Field{Key: "error", Value: err})
		}
	}()
	go func() {
		if err := c.k8sClient.WatchPods(ctx, c.podEvents); err != nil {
			c.logger.Error("Failed to watch pods", logging.Field{Key: "error", Value: err})
		}
	}()
	go func() {
		if err := c.k8sClient.WatchNodes(ctx, c.nodeEvents); err != nil {
			c.logger.Error("Failed to watch nodes", logging.Field{Key: "error", Value: err})
		}
	}()

	for i := 0; i < c.config.RequestPoolSize; i++ {
		go wait.UntilWithContext(ctx, c.runWorker, time.Second)
	}

	c.syncTicker = time.NewTicker(c.config.GlobalUpstreamSyncPeriod)
	go c.runPeriodicSync(ctx)
	go c.processEvents(ctx)

	if err := c.SyncUpstreams(ctx); err != nil {
		c.logger.Error("Initial bulk upstream sync failed", logging.Field{Key: "error", Value: err})
	}

	<-ctx.Done()
	c.logger.Info("Controller stopped")
	c.stop()
	return ctx.Err()
}

func (c *ControllerImpl) stop() {
	if c.syncTicker != nil {
		c.syncTicker.Stop()
	}
	close(c.syncStop)
	c.queue.ShutDown()
}

func (c *ControllerImpl) runWorker(ctx context.Context) {
	for c.processNextItem(ctx) {
	}
}

func (c *ControllerImpl) processNextItem(ctx context.Context) bool {
	obj, quit := c.queue.Get()
	if quit {
		return false
	}
	defer c.queue.Done(obj)

	key, ok := obj.(string)
	if !ok {
		c.queue.Forget(obj)
		return true
	}

	err := c.syncHandler(ctx, key)
	if err == nil {
		c.queue.Forget(obj)
		return true
	}

	c.logger.Error("Error syncing upstream, backing off", logging.Field{Key: "key", Value: key}, logging.Field{Key: "error", Value: err})
	c.queue.AddRateLimited(obj)
	return true
}

func (c *ControllerImpl) syncHandler(ctx context.Context, key string) error {
	c.stateMutex.RLock()
	svc, exists := c.state.Services[key]
	lastSvc, hadLast := c.lastKnownServices[key]
	c.stateMutex.RUnlock()

	if !exists {
		if !hadLast {
			return nil
		}
		if err := c.handleServiceDeletion(ctx, key, lastSvc); err != nil {
			return err
		}
		c.stateMutex.Lock()
		delete(c.lastKnownServices, key)
		c.stateMutex.Unlock()
		return nil
	}

	c.stateMutex.Lock()
	c.lastKnownServices[key] = svc
	c.stateMutex.Unlock()

	return c.syncUpstreamsForService(ctx, svc)
}

func (c *ControllerImpl) handleServiceDeletion(ctx context.Context, key string, svc model.Service) error {
	lbName := svc.Labels["mlb-loadbalancer-name"]
	upstreamName := svc.Labels["mlb-upstream-name"]

	if lbName == "" || upstreamName == "" {
		return nil
	}

	// Check if another Service in the cluster still targets this (lbName, upstreamName)
	c.stateMutex.RLock()
	var otherSvc *model.Service
	for _, s := range c.state.Services {
		if s.Labels["mlb-loadbalancer-name"] == lbName && s.Labels["mlb-upstream-name"] == upstreamName {
			copied := s
			otherSvc = &copied
			break
		}
	}
	c.stateMutex.RUnlock()

	if otherSvc != nil {
		// Another service still targets the upstream; resync with its state
		return c.syncUpstreamsForService(ctx, *otherSvc)
	}

	// No other Service targets this upstream; purge it from state and drain on the LB
	lbConfig, _ := c.getLoadBalancerConfig(lbName)
	emptyUpstream := &model.Upstream{
		Name:         upstreamName,
		LoadBalancer: lbName,
		Backends:     []model.Backend{},
		Type:         c.getLoadBalancerType(lbName),
		Config:       lbConfig,
	}

	c.stateMutex.Lock()
	delete(c.state.Upstreams, fmt.Sprintf("%s/%s", lbName, upstreamName))
	c.stateMutex.Unlock()

	c.metrics.UpdateBackendsTotal(upstreamName, 0)

	if err := c.syncLoadBalancerUpstream(ctx, emptyUpstream); err != nil {
		c.metrics.IncrementSyncErrors(upstreamName)
		return err
	}

	return nil
}

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

func (c *ControllerImpl) handleServiceEvent(event kubernetes.ServiceEvent) {
	key := fmt.Sprintf("%s/%s", event.Service.Namespace, event.Service.Name)
	c.stateMutex.Lock()
	switch event.Type {
	case kubernetes.EventTypeAdd, kubernetes.EventTypeUpdate:
		c.state.Services[key] = event.Service
		c.lastKnownServices[key] = event.Service
	case kubernetes.EventTypeDelete:
		delete(c.state.Services, key)
	}
	c.stateMutex.Unlock()

	c.queue.Add(key)
}

func (c *ControllerImpl) handlePodEvent(event kubernetes.PodEvent) {
	key := fmt.Sprintf("%s/%s", event.Pod.Namespace, event.Pod.Name)
	c.stateMutex.Lock()
	switch event.Type {
	case kubernetes.EventTypeAdd, kubernetes.EventTypeUpdate:
		c.state.Pods[key] = event.Pod
	case kubernetes.EventTypeDelete:
		delete(c.state.Pods, key)
	}
	c.stateMutex.Unlock()

	c.enqueueUpstreamsForPod(event.Pod)
}

func (c *ControllerImpl) handleNodeEvent(event kubernetes.NodeEvent) {
	key := event.Node.Name
	c.stateMutex.Lock()
	switch event.Type {
	case kubernetes.EventTypeAdd, kubernetes.EventTypeUpdate:
		c.state.Nodes[key] = event.Node
	case kubernetes.EventTypeDelete:
		delete(c.state.Nodes, key)
	}
	c.stateMutex.Unlock()

	c.enqueueUpstreamsForNode(event.Node)
}

func (c *ControllerImpl) enqueueUpstreamsForPod(pod model.Pod) {
	c.stateMutex.RLock()
	defer c.stateMutex.RUnlock()
	for _, svc := range c.state.Services {
		if c.matchLabels(pod.Labels, svc.Selector) {
			c.queue.Add(fmt.Sprintf("%s/%s", svc.Namespace, svc.Name))
		}
	}
}

func (c *ControllerImpl) enqueueUpstreamsForNode(node model.Node) {
	c.stateMutex.RLock()
	var affectedPods []model.Pod
	for _, pod := range c.state.Pods {
		if pod.NodeName == node.Name {
			affectedPods = append(affectedPods, pod)
		}
	}
	c.stateMutex.RUnlock()

	for _, pod := range affectedPods {
		c.enqueueUpstreamsForPod(pod)
	}
}

func (c *ControllerImpl) syncUpstreamsForService(ctx context.Context, svc model.Service) error {
	startTime := time.Now()
	upstreamName := svc.Labels["mlb-upstream-name"]
	lbName := svc.Labels["mlb-loadbalancer-name"]

	if lbName == "" || upstreamName == "" {
		return nil
	}

	pods, err := c.k8sClient.GetPodsForSelector(ctx, svc.Namespace, svc.Selector)
	if err != nil {
		c.metrics.IncrementSyncErrors(upstreamName)
		return fmt.Errorf("failed to get pods for service %s/%s: %w", svc.Namespace, svc.Name, err)
	}

	upstream := c.buildUpstream(ctx, svc, pods)
	if upstream == nil {
		c.metrics.UpdateBackendsTotal(upstreamName, 0)
		return nil
	}

	c.metrics.UpdateBackendsTotal(upstreamName, float64(len(upstream.Backends)))

	key := fmt.Sprintf("%s/%s", lbName, upstreamName)

	c.stateMutex.Lock()
	c.state.Upstreams[key] = *upstream
	c.stateMutex.Unlock()

	if err := c.syncLoadBalancerUpstream(ctx, upstream); err != nil {
		c.metrics.IncrementSyncErrors(upstreamName)
		return err
	}

	c.metrics.ObserveSyncDuration(upstreamName, time.Since(startTime).Seconds())
	return nil
}

// اجرای مستقیم Two-Way Reconciliation در بازه‌های زمانی ثابت
func (c *ControllerImpl) runPeriodicSync(ctx context.Context) {
	if c.syncTicker == nil {
		c.syncTicker = time.NewTicker(c.config.GlobalUpstreamSyncPeriod)
	}
	for {
		select {
		case <-c.syncTicker.C:
			if err := c.SyncUpstreams(ctx); err != nil {
				c.logger.Error("Periodic drift reconciliation failed", logging.Field{Key: "error", Value: err})
			}
		case <-c.syncStop:
			return
		case <-ctx.Done():
			return
		}
	}
}

// نگاشت کامل State مطلوب K8s و ارسال به لودبالانسرها جهت حذف Stateهای Drift شده
func (c *ControllerImpl) SyncUpstreams(ctx context.Context) error {
	startTime := time.Now()

	c.stateMutex.RLock()
	services := make([]model.Service, 0, len(c.state.Services))
	for _, svc := range c.state.Services {
		services = append(services, svc)
	}
	c.stateMutex.RUnlock()

	desiredState := make(map[string]map[string]model.Upstream)
	for lbName := range c.loadBalancers {
		desiredState[lbName] = make(map[string]model.Upstream)
	}

	for _, svc := range services {
		lbName := svc.Labels["mlb-loadbalancer-name"]
		if lbName == "" {
			continue
		}

		pods, err := c.k8sClient.GetPodsForSelector(ctx, svc.Namespace, svc.Selector)
		if err != nil {
			c.logger.Error("Failed to list pods for bulk reconciliation", logging.Field{Key: "service", Value: svc.Name})
			continue
		}

		if upstream := c.buildUpstream(ctx, svc, pods); upstream != nil {
			if _, exists := desiredState[lbName]; !exists {
				desiredState[lbName] = make(map[string]model.Upstream)
			}
			desiredState[lbName][upstream.Name] = *upstream
		}
	}

	// بروزرسانی Cache کنترلر
	c.stateMutex.Lock()
	c.state.Upstreams = make(map[string]model.Upstream)
	for lbName, upstreams := range desiredState {
		for upName, up := range upstreams {
			c.state.Upstreams[fmt.Sprintf("%s/%s", lbName, upName)] = up
		}
	}
	c.stateMutex.Unlock()

	var errs []error
	for lbName, adapter := range c.loadBalancers {
		if err := adapter.SyncAllUpstreams(ctx, desiredState[lbName]); err != nil {
			errs = append(errs, fmt.Errorf("reconciliation failed for LB %s: %w", lbName, err))
		}
	}

	c.syncUpstreamCtr.Inc()
	c.metrics.ObserveSyncDuration("two_way_reconciliation", time.Since(startTime).Seconds())

	if len(errs) > 0 {
		c.metrics.IncrementSyncErrors("two_way_reconciliation")
		return fmt.Errorf("drift reconciliation errors: %v", errs)
	}

	return nil
}

func (c *ControllerImpl) buildUpstream(ctx context.Context, svc model.Service, pods []model.Pod) *model.Upstream {
	lbName := svc.Labels["mlb-loadbalancer-name"]
	upstreamName := svc.Labels["mlb-upstream-name"]
	if svc.NodePort == 0 || lbName == "" || upstreamName == "" {
		return nil
	}

	backends := make([]model.Backend, 0)
	for _, pod := range pods {
		if !pod.Ready || pod.NodeName == "" {
			continue
		}

		node, err := c.k8sClient.GetNodeByName(ctx, pod.NodeName)
		if err != nil {
			continue
		}

		nodeIP := node.InternalIP
		if nodeIP == "" {
			nodeIP = node.ExternalIP
		}
		if nodeIP == "" {
			continue
		}

		lbConfig, exists := c.getLoadBalancerConfig(lbName)
		if exists && lbConfig.GetIPReplacement() {
			nodeIP = util.ApplyIPReplacement(nodeIP, lbConfig, c.config.GlobalIPReplacementList)
		}

		backends = append(backends, model.Backend{
			IP:     nodeIP,
			Port:   svc.NodePort,
			Weight: 1,
		})
	}

	lbConfig, _ := c.getLoadBalancerConfig(lbName)
	return &model.Upstream{
		Name:         upstreamName,
		LoadBalancer: lbName,
		Backends:     backends,
		Type:         c.getLoadBalancerType(lbName),
		Config:       lbConfig,
	}
}

func (c *ControllerImpl) syncLoadBalancerUpstream(ctx context.Context, upstream *model.Upstream) error {
	adapter, exists := c.loadBalancers[upstream.LoadBalancer]
	if !exists {
		return fmt.Errorf("load balancer adapter not found: %s", upstream.LoadBalancer)
	}

	if err := adapter.SyncUpstream(ctx, *upstream); err != nil {
		return fmt.Errorf("failed to sync upstream %s/%s: %w", upstream.LoadBalancer, upstream.Name, err)
	}

	c.logger.Info("Upstream synced successfully",
		logging.Field{Key: "loadbalancer", Value: upstream.LoadBalancer},
		logging.Field{Key: "upstream", Value: upstream.Name},
		logging.Field{Key: "backends", Value: len(upstream.Backends)})
	return nil
}

func (c *ControllerImpl) getLoadBalancerConfig(lbName string) (config.LoadBalancerConfig, bool) {
	for _, lb := range c.config.LoadBalancers.LoadBalancers {
		if lb.GetName() == lbName {
			return lb, true
		}
	}
	return nil, false
}

func (c *ControllerImpl) getLoadBalancerType(lbName string) string {
	for _, lb := range c.config.LoadBalancers.LoadBalancers {
		if lb.GetName() == lbName {
			return lb.GetType()
		}
	}
	return ""
}

func (c *ControllerImpl) matchLabels(podLabels, svcSelector map[string]string) bool {
	if len(svcSelector) == 0 {
		return false
	}
	for k, v := range svcSelector {
		if podLabels[k] != v {
			return false
		}
	}
	return true
}

func (c *ControllerImpl) GetState() model.ControllerState {
	c.stateMutex.RLock()
	defer c.stateMutex.RUnlock()

	services := make(map[string]model.Service, len(c.state.Services))
	for k, v := range c.state.Services {
		services[k] = v
	}
	pods := make(map[string]model.Pod, len(c.state.Pods))
	for k, v := range c.state.Pods {
		pods[k] = v
	}
	nodes := make(map[string]model.Node, len(c.state.Nodes))
	for k, v := range c.state.Nodes {
		nodes[k] = v
	}
	upstreams := make(map[string]model.Upstream, len(c.state.Upstreams))
	for k, v := range c.state.Upstreams {
		upstreams[k] = v
	}

	return model.ControllerState{
		Services:  services,
		Pods:      pods,
		Nodes:     nodes,
		Upstreams: upstreams,
	}
}
