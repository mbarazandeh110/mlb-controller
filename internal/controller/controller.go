// internal/controller/controller.go
package controller

import (
	"context"
	"fmt"
	"sort"
	"strings"
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
		syncUpstreamCtr:   metrics.NewCounter("mlb_sync_upstreams_total", "Total number of upstream sync operations", nil),
		queue:             workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "mlb_sync_queue"),
	}
}

func (c *ControllerImpl) Start(ctx context.Context) error {
	c.logger.Info("Starting controller with Two-Way Reconciliation Engine")

	// Start event processing loop first so channel buffers never fill/block during initial cache sync
	go c.processEvents(ctx)

	// Register event handlers synchronously before starting informers to avoid any race condition
	if err := c.k8sClient.WatchServices(ctx, nil, c.serviceEvents); err != nil {
		return fmt.Errorf("failed to register service watcher: %w", err)
	}
	if err := c.k8sClient.WatchPods(ctx, c.podEvents); err != nil {
		return fmt.Errorf("failed to register pod watcher: %w", err)
	}
	if err := c.k8sClient.WatchNodes(ctx, c.nodeEvents); err != nil {
		return fmt.Errorf("failed to register node watcher: %w", err)
	}

	if err := c.k8sClient.Start(ctx); err != nil {
		return fmt.Errorf("failed to start and sync kubernetes informers: %w", err)
	}

	for i := 0; i < c.config.RequestPoolSize; i++ {
		go wait.UntilWithContext(ctx, c.runWorker, time.Second)
	}

	go c.runPeriodicSync(ctx)

	if err := c.SyncUpstreams(ctx); err != nil {
		c.logger.Error("Initial bulk upstream sync failed", logging.Field{Key: "error", Value: err})
	}

	<-ctx.Done()
	c.logger.Info("Controller stopped")
	c.stop()
	return ctx.Err()
}

func (c *ControllerImpl) stop() {
	select {
	case <-c.syncStop:
	default:
		close(c.syncStop)
	}
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
	parts := strings.Split(key, "/")
	if len(parts) != 2 {
		return fmt.Errorf("invalid resource key: %s", key)
	}
	namespace, name := parts[0], parts[1]

	svc, err := c.k8sClient.GetService(namespace, name)
	if err != nil {
		// Service deleted or no longer managed
		c.stateMutex.RLock()
		lastSvc, hadLast := c.lastKnownServices[key]
		c.stateMutex.RUnlock()

		if !hadLast {
			return nil
		}
		if err := c.handleServiceDeletion(ctx, key, lastSvc); err != nil {
			return err
		}
		c.stateMutex.Lock()
		delete(c.lastKnownServices, key)
		delete(c.state.Services, key)
		c.stateMutex.Unlock()
		return nil
	}

	c.stateMutex.RLock()
	lastSvc, hadLast := c.lastKnownServices[key]
	c.stateMutex.RUnlock()

	if hadLast {
		oldLB := lastSvc.Labels["mlb-loadbalancer-name"]
		oldUp := lastSvc.Labels["mlb-upstream-name"]
		newLB := svc.Labels["mlb-loadbalancer-name"]
		newUp := svc.Labels["mlb-upstream-name"]

		if (oldLB != newLB || oldUp != newUp) && (oldLB != "" && oldUp != "") {
			// Upstream or LB changed for this service. Drain the old upstream!
			if err := c.handleServiceDeletion(ctx, key, lastSvc); err != nil {
				c.logger.Error("Failed to cleanup old upstream on service label change",
					logging.Field{Key: "key", Value: key},
					logging.Field{Key: "old_lb", Value: oldLB},
					logging.Field{Key: "old_upstream", Value: oldUp},
					logging.Field{Key: "error", Value: err})
				return err
			}
		}
	}

	c.stateMutex.Lock()
	c.lastKnownServices[key] = *svc
	c.state.Services[key] = *svc
	c.stateMutex.Unlock()

	return c.syncUpstreamsForService(ctx, *svc)
}

func (c *ControllerImpl) handleServiceDeletion(ctx context.Context, key string, svc model.Service) error {
	lbName := svc.Labels["mlb-loadbalancer-name"]
	upstreamName := svc.Labels["mlb-upstream-name"]

	if lbName == "" || upstreamName == "" {
		return nil
	}

	// Check if another Service in the cluster still targets this (lbName, upstreamName)
	services, err := c.k8sClient.ListServices()
	if err != nil {
		return fmt.Errorf("failed to list services during service deletion check for %s: %w", key, err)
	}
	var otherSvc *model.Service
	for _, s := range services {
		sKey := fmt.Sprintf("%s/%s", s.Namespace, s.Name)
		if sKey != key && s.Labels["mlb-loadbalancer-name"] == lbName && s.Labels["mlb-upstream-name"] == upstreamName {
			copied := s
			otherSvc = &copied
			break
		}
	}

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
	case kubernetes.EventTypeAdd:
		c.state.Services[key] = event.Service
		if _, exists := c.lastKnownServices[key]; !exists {
			c.lastKnownServices[key] = event.Service
		}
	case kubernetes.EventTypeUpdate:
		c.state.Services[key] = event.Service
		// Note: We deliberately do not overwrite c.lastKnownServices[key] here
		// so that syncHandler can detect differences (LB, upstream name changes) between previous and current state.
	case kubernetes.EventTypeDelete:
		delete(c.state.Services, key)
	}
	c.stateMutex.Unlock()

	c.queue.Add(key)
}

func (c *ControllerImpl) handlePodEvent(event kubernetes.PodEvent) {
	key := fmt.Sprintf("%s/%s", event.Pod.Namespace, event.Pod.Name)
	c.stateMutex.Lock()
	oldPod, hadOldPod := c.state.Pods[key]
	switch event.Type {
	case kubernetes.EventTypeAdd, kubernetes.EventTypeUpdate:
		c.state.Pods[key] = event.Pod
	case kubernetes.EventTypeDelete:
		delete(c.state.Pods, key)
	}
	c.stateMutex.Unlock()

	if hadOldPod && event.Type == kubernetes.EventTypeUpdate {
		c.enqueueUpstreamsForPod(oldPod)
	}
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
		if svc.Namespace == pod.Namespace && c.matchLabels(pod.Labels, svc.Selector) {
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

	upstream, err := c.buildUpstream(ctx, svc, pods)
	if err != nil {
		c.metrics.IncrementSyncErrors(upstreamName)
		return fmt.Errorf("failed to build upstream for service %s/%s: %w", svc.Namespace, svc.Name, err)
	}
	if upstream == nil {
		return nil
	}

	// Merge backends from other Services that target this same (lbName, upstreamName)
	allServices, err := c.k8sClient.ListServices()
	if err != nil {
		c.metrics.IncrementSyncErrors(upstreamName)
		return fmt.Errorf("failed to list services for upstream merge (%s/%s): %w", lbName, upstreamName, err)
	}

	for _, otherSvc := range allServices {
		if otherSvc.Namespace == svc.Namespace && otherSvc.Name == svc.Name {
			continue
		}
		if otherSvc.Labels["mlb-loadbalancer-name"] == lbName && otherSvc.Labels["mlb-upstream-name"] == upstreamName {
			otherPods, err := c.k8sClient.GetPodsForSelector(ctx, otherSvc.Namespace, otherSvc.Selector)
			if err != nil {
				c.metrics.IncrementSyncErrors(upstreamName)
				return fmt.Errorf("failed to get pods for shared service %s/%s: %w", otherSvc.Namespace, otherSvc.Name, err)
			}
			otherUp, err := c.buildUpstream(ctx, otherSvc, otherPods)
			if err != nil {
				c.metrics.IncrementSyncErrors(upstreamName)
				return fmt.Errorf("failed to build upstream for shared service %s/%s: %w", otherSvc.Namespace, otherSvc.Name, err)
			}
			if otherUp != nil {
				upstream.Backends = c.mergeBackends(upstream.Backends, otherUp.Backends)
			}
		}
	}

	lbConfig, _ := c.getLoadBalancerConfig(lbName)

	// Check if backends is empty (no pods or no ready pods)
	if len(upstream.Backends) == 0 {
		var defBackend *config.DefaultBackendConfig
		if lbConfig != nil && lbConfig.GetDefaultBackend() != nil {
			defBackend = lbConfig.GetDefaultBackend()
		} else if c.config.DefaultBackend != nil {
			defBackend = c.config.DefaultBackend
		}

		if defBackend != nil && defBackend.IP != "" {
			w := defBackend.Weight
			if w <= 0 {
				w = 1
			}
			upstream.Backends = []model.Backend{
				{
					IP:     defBackend.IP,
					Port:   defBackend.Port,
					Weight: w,
				},
			}
			c.logger.Info("Using fallback default backend because no ready pods were found",
				logging.Field{Key: "service", Value: fmt.Sprintf("%s/%s", svc.Namespace, svc.Name)},
				logging.Field{Key: "upstream", Value: upstreamName},
				logging.Field{Key: "default_ip", Value: defBackend.IP},
				logging.Field{Key: "default_port", Value: defBackend.Port})
		} else {
			allowEmpty := c.config.AllowEmptyUpstream
			if lbConfig != nil && lbConfig.GetAllowEmptyUpstream() {
				allowEmpty = true
			}
			if !allowEmpty {
				c.logger.Warn("Safety Net triggered: no ready pods found and allow_empty_upstream is false; preserving existing backends",
					logging.Field{Key: "service", Value: fmt.Sprintf("%s/%s", svc.Namespace, svc.Name)},
					logging.Field{Key: "upstream", Value: upstreamName},
					logging.Field{Key: "loadbalancer", Value: lbName})
				return nil
			}
		}
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

// اجرای مستقیم Two-Way Reconciliation در بازه‌های زمانی اختصاصی یا سراسری
func (c *ControllerImpl) runPeriodicSync(ctx context.Context) {
	if len(c.loadBalancers) == 0 {
		ticker := time.NewTicker(c.config.GlobalUpstreamSyncPeriod)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
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

	var wg sync.WaitGroup
	for lbName, adapter := range c.loadBalancers {
		period := c.config.GlobalUpstreamSyncPeriod
		if lbConfig, ok := c.getLoadBalancerConfig(lbName); ok && lbConfig.GetUpstreamSyncPeriod() > 0 {
			period = lbConfig.GetUpstreamSyncPeriod()
		}
		if period <= 0 {
			period = 30 * time.Second
		}

		wg.Add(1)
		go func(name string, ad loadbalancer.LoadBalancerAdapter, p time.Duration) {
			defer wg.Done()
			ticker := time.NewTicker(p)
			defer ticker.Stop()

			for {
				select {
				case <-ticker.C:
					if err := c.syncSingleLoadBalancer(ctx, name, ad); err != nil {
						c.logger.Error("Periodic drift reconciliation failed for load balancer",
							logging.Field{Key: "loadbalancer", Value: name},
							logging.Field{Key: "error", Value: err})
					}
				case <-c.syncStop:
					return
				case <-ctx.Done():
					return
				}
			}
		}(lbName, adapter, period)
	}
	wg.Wait()
}

func (c *ControllerImpl) syncSingleLoadBalancer(ctx context.Context, lbName string, adapter loadbalancer.LoadBalancerAdapter) error {
	startTime := time.Now()

	if !c.HasSynced() {
		c.logger.Warn("Skipping sync because informer caches are not yet synchronized",
			logging.Field{Key: "loadbalancer", Value: lbName})
		return nil
	}

	services, err := c.k8sClient.ListServices()
	if err != nil {
		return fmt.Errorf("failed to list services from lister for LB %s: %w", lbName, err)
	}

	// Populate/sync c.state.Services and c.lastKnownServices for this loadbalancer
	currentSvcKeys := make(map[string]struct{})
	c.stateMutex.Lock()
	for _, svc := range services {
		if svc.Labels["mlb-loadbalancer-name"] != lbName {
			continue
		}
		key := fmt.Sprintf("%s/%s", svc.Namespace, svc.Name)
		currentSvcKeys[key] = struct{}{}
		c.state.Services[key] = svc
		if _, exists := c.lastKnownServices[key]; !exists {
			c.lastKnownServices[key] = svc
		}
	}
	for key, svc := range c.state.Services {
		if svc.Labels["mlb-loadbalancer-name"] == lbName {
			if _, exists := currentSvcKeys[key]; !exists {
				delete(c.state.Services, key)
			}
		}
	}
	c.stateMutex.Unlock()

	desiredState := make(map[string]model.Upstream)

	for _, svc := range services {
		if svc.Labels["mlb-loadbalancer-name"] != lbName {
			continue
		}

		upstreamName := svc.Labels["mlb-upstream-name"]
		if upstreamName == "" {
			continue
		}

		pods, err := c.k8sClient.GetPodsForSelector(ctx, svc.Namespace, svc.Selector)
		if err != nil {
			c.logger.Error("Failed to list pods for reconciliation; preserving upstream (fail-static)",
				logging.Field{Key: "service", Value: svc.Name},
				logging.Field{Key: "upstream", Value: upstreamName},
				logging.Field{Key: "error", Value: err})
			preserveUp := model.Upstream{
				Name:                     upstreamName,
				LoadBalancer:             lbName,
				PreserveExistingBackends: true,
			}
			c.stateMutex.RLock()
			if oldUp, ok := c.state.Upstreams[fmt.Sprintf("%s/%s", lbName, upstreamName)]; ok {
				preserveUp.Backends = oldUp.Backends
			}
			c.stateMutex.RUnlock()
			desiredState[upstreamName] = preserveUp
			continue
		}

		upstream, err := c.buildUpstream(ctx, svc, pods)
		if err != nil {
			c.logger.Error("Failed to build upstream for reconciliation; preserving upstream (fail-static)",
				logging.Field{Key: "service", Value: svc.Name},
				logging.Field{Key: "upstream", Value: upstreamName},
				logging.Field{Key: "error", Value: err})
			preserveUp := model.Upstream{
				Name:                     upstreamName,
				LoadBalancer:             lbName,
				PreserveExistingBackends: true,
			}
			c.stateMutex.RLock()
			if oldUp, ok := c.state.Upstreams[fmt.Sprintf("%s/%s", lbName, upstreamName)]; ok {
				preserveUp.Backends = oldUp.Backends
			}
			c.stateMutex.RUnlock()
			desiredState[upstreamName] = preserveUp
			continue
		}
		if upstream == nil {
			continue
		}

		if existingUp, exists := desiredState[upstream.Name]; exists {
			if !existingUp.PreserveExistingBackends {
				existingUp.Backends = c.mergeBackends(existingUp.Backends, upstream.Backends)
				desiredState[upstream.Name] = existingUp
			}
		} else {
			desiredState[upstream.Name] = *upstream
		}
	}

	lbConfig, _ := c.getLoadBalancerConfig(lbName)

	// Apply fallback default backend or safety net for upstreams with zero backends
	for upName, up := range desiredState {
		if up.PreserveExistingBackends {
			continue
		}
		if len(up.Backends) == 0 {
			var defBackend *config.DefaultBackendConfig
			if lbConfig != nil && lbConfig.GetDefaultBackend() != nil {
				defBackend = lbConfig.GetDefaultBackend()
			} else if c.config.DefaultBackend != nil {
				defBackend = c.config.DefaultBackend
			}

			if defBackend != nil && defBackend.IP != "" {
				w := defBackend.Weight
				if w <= 0 {
					w = 1
				}
				up.Backends = []model.Backend{
					{
						IP:     defBackend.IP,
						Port:   defBackend.Port,
						Weight: w,
					},
				}
				desiredState[upName] = up
			} else {
				allowEmpty := c.config.AllowEmptyUpstream
				if lbConfig != nil && lbConfig.GetAllowEmptyUpstream() {
					allowEmpty = true
				}
				if !allowEmpty {
					// Safety net: preserve existing state from c.state if present, and mark Preserved so LB is untouched
					c.stateMutex.RLock()
					oldUp, exists := c.state.Upstreams[fmt.Sprintf("%s/%s", lbName, upName)]
					c.stateMutex.RUnlock()
					if exists && len(oldUp.Backends) > 0 {
						up.Backends = oldUp.Backends
					}
					up.PreserveExistingBackends = true
					desiredState[upName] = up
				}
			}
		}
	}

	// Update c.state.Upstreams and metrics, cleaning up removed upstreams
	c.stateMutex.Lock()
	prefix := lbName + "/"
	for k := range c.state.Upstreams {
		if strings.HasPrefix(k, prefix) {
			upName := strings.TrimPrefix(k, prefix)
			if _, exists := desiredState[upName]; !exists {
				delete(c.state.Upstreams, k)
				c.metrics.UpdateBackendsTotal(upName, 0)
			}
		}
	}
	for upName, up := range desiredState {
		if !up.PreserveExistingBackends {
			c.state.Upstreams[fmt.Sprintf("%s/%s", lbName, upName)] = up
			c.metrics.UpdateBackendsTotal(upName, float64(len(up.Backends)))
		}
	}
	c.stateMutex.Unlock()

	if err := adapter.SyncAllUpstreams(ctx, desiredState); err != nil {
		c.metrics.IncrementSyncErrors(lbName)
		return fmt.Errorf("reconciliation failed for LB %s: %w", lbName, err)
	}

	c.syncUpstreamCtr.Inc()
	c.metrics.ObserveSyncDuration(fmt.Sprintf("reconciliation_%s", lbName), time.Since(startTime).Seconds())
	return nil
}

// نگاشت کامل State مطلوب K8s و ارسال به لودبالانسرها جهت حذف Stateهای Drift شده
func (c *ControllerImpl) SyncUpstreams(ctx context.Context) error {
	var errs []error
	for lbName, adapter := range c.loadBalancers {
		if err := c.syncSingleLoadBalancer(ctx, lbName, adapter); err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("drift reconciliation errors: %v", errs)
	}

	return nil
}

func (c *ControllerImpl) buildUpstream(ctx context.Context, svc model.Service, pods []model.Pod) (*model.Upstream, error) {
	lbName := svc.Labels["mlb-loadbalancer-name"]
	upstreamName := svc.Labels["mlb-upstream-name"]
	if svc.NodePort == 0 || lbName == "" || upstreamName == "" {
		return nil, nil
	}

	backends := make([]model.Backend, 0)
	for _, pod := range pods {
		if !pod.Ready || pod.NodeName == "" {
			continue
		}

		node, err := c.k8sClient.GetNodeByName(ctx, pod.NodeName)
		if err != nil {
			return nil, fmt.Errorf("failed to get node %s for pod %s: %w", pod.NodeName, pod.Name, err)
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
		Backends:     c.mergeBackends(backends, nil),
		Type:         c.getLoadBalancerType(lbName),
		Config:       lbConfig,
	}, nil
}

func (c *ControllerImpl) mergeBackends(a, b []model.Backend) []model.Backend {
	m := make(map[string]model.Backend)
	for _, be := range a {
		key := fmt.Sprintf("%s:%d", be.IP, be.Port)
		if existing, ok := m[key]; ok {
			existing.Weight += be.Weight
			m[key] = existing
		} else {
			m[key] = be
		}
	}
	for _, be := range b {
		key := fmt.Sprintf("%s:%d", be.IP, be.Port)
		if existing, ok := m[key]; ok {
			existing.Weight += be.Weight
			m[key] = existing
		} else {
			m[key] = be
		}
	}
	res := make([]model.Backend, 0, len(m))
	for _, be := range m {
		res = append(res, be)
	}
	sort.Slice(res, func(i, j int) bool {
		if res[i].IP != res[j].IP {
			return res[i].IP < res[j].IP
		}
		return res[i].Port < res[j].Port
	})
	return res
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

// HasSynced returns true if the controller's underlying informers have synchronized.
func (c *ControllerImpl) HasSynced() bool {
	if c.k8sClient == nil {
		return false
	}
	return c.k8sClient.HasSynced()
}

// StartInformers starts the underlying informers independently of leadership.
func (c *ControllerImpl) StartInformers(ctx context.Context) error {
	if c.k8sClient == nil {
		return nil
	}
	return c.k8sClient.Start(ctx)
}
