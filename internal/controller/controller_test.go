package controller

import (
	"context"
	"sync"
	"testing"
	"time"

	"mlb-controller/internal/domain/config"
	"mlb-controller/internal/domain/model"
	"mlb-controller/internal/ports/kubernetes"
	"mlb-controller/internal/ports/loadbalancer"
	"mlb-controller/internal/ports/logging"
	"mlb-controller/internal/ports/metrics"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeLogger struct{}

func (f *fakeLogger) Debug(msg string, fields ...logging.Field)   {}
func (f *fakeLogger) Info(msg string, fields ...logging.Field)    {}
func (f *fakeLogger) Warn(msg string, fields ...logging.Field)    {}
func (f *fakeLogger) Error(msg string, fields ...logging.Field)   {}
func (f *fakeLogger) Fatal(msg string, fields ...logging.Field)   {}
func (f *fakeLogger) With(fields ...logging.Field) logging.Logger { return f }
func (f *fakeLogger) Sync() error                                 { return nil }

type fakeMetrics struct {
	mu             sync.Mutex
	backendsTotals map[string]float64
	syncErrors     map[string]int
}

func newFakeMetrics() *fakeMetrics {
	return &fakeMetrics{
		backendsTotals: make(map[string]float64),
		syncErrors:     make(map[string]int),
	}
}

func (f *fakeMetrics) NewCounter(name, help string, labels []string) metrics.Counter {
	return &fakeCounter{}
}
func (f *fakeMetrics) NewGauge(name, help string, labels []string) metrics.Gauge { return nil }
func (f *fakeMetrics) NewHistogram(name, help string, labels []string) metrics.Histogram {
	return &fakeHistogram{}
}
func (f *fakeMetrics) Start() error                            { return nil }
func (f *fakeMetrics) Stop(ctx context.Context) error          { return nil }
func (f *fakeMetrics) ProxyToLeader(ctx context.Context) error { return nil }
func (f *fakeMetrics) UpdateBackendsTotal(upstream string, count float64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.backendsTotals[upstream] = count
}
func (f *fakeMetrics) IncrementSyncErrors(upstream string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.syncErrors[upstream]++
}
func (f *fakeMetrics) ObserveSyncDuration(upstream string, duration float64) {}

type fakeCounter struct{}

func (f *fakeCounter) Inc()              {}
func (f *fakeCounter) Add(delta float64) {}

type fakeHistogram struct{}

func (f *fakeHistogram) Observe(value float64) {}

type fakeLBAdapter struct {
	mu             sync.Mutex
	syncedUpstream []model.Upstream
	allUpstreams   map[string]model.Upstream
}

func newFakeLBAdapter() *fakeLBAdapter {
	return &fakeLBAdapter{
		allUpstreams: make(map[string]model.Upstream),
	}
}

func (f *fakeLBAdapter) SyncUpstream(ctx context.Context, upstream model.Upstream) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.syncedUpstream = append(f.syncedUpstream, upstream)
	f.allUpstreams[upstream.Name] = upstream
	return nil
}

func (f *fakeLBAdapter) SyncAllUpstreams(ctx context.Context, desired map[string]model.Upstream) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.allUpstreams = desired
	return nil
}

type fakeK8sPort struct {
	mu       sync.Mutex
	pods     []model.Pod
	nodes    map[string]model.Node
	podError error
}

func (f *fakeK8sPort) WatchServices(ctx context.Context, requiredLabels map[string]string, events chan<- kubernetes.ServiceEvent) error {
	<-ctx.Done()
	return ctx.Err()
}

func (f *fakeK8sPort) WatchPods(ctx context.Context, events chan<- kubernetes.PodEvent) error {
	<-ctx.Done()
	return ctx.Err()
}

func (f *fakeK8sPort) WatchNodes(ctx context.Context, events chan<- kubernetes.NodeEvent) error {
	<-ctx.Done()
	return ctx.Err()
}

func (f *fakeK8sPort) GetPodsForSelector(ctx context.Context, namespace string, selector map[string]string) ([]model.Pod, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.podError != nil {
		return nil, f.podError
	}
	return f.pods, nil
}

func (f *fakeK8sPort) GetNodeByName(ctx context.Context, name string) (model.Node, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	node, exists := f.nodes[name]
	if !exists {
		return model.Node{Name: name, InternalIP: "192.168.1.100"}, nil
	}
	return node, nil
}

func setupTestController(t *testing.T) (*ControllerImpl, *fakeLBAdapter, *fakeK8sPort, *fakeMetrics) {
	lbAdapter := newFakeLBAdapter()
	lbMap := map[string]loadbalancer.LoadBalancerAdapter{
		"test-lb": lbAdapter,
	}

	cfg := &config.Config{
		RequestPoolSize:          2,
		GlobalUpstreamSyncPeriod: 10 * time.Minute,
		GlobalIPReplacementList:  config.GlobalIPReplacementList{},
		LoadBalancers: config.LoadBalancersConfig{
			LoadBalancers: []config.LoadBalancerConfig{
				&config.NginxConfig{
					Type: "nginx",
					Name: "test-lb",
				},
			},
		},
	}

	k8sPort := &fakeK8sPort{
		nodes: map[string]model.Node{
			"node1": {Name: "node1", InternalIP: "192.168.1.10"},
			"node2": {Name: "node2", InternalIP: "192.168.1.20"},
		},
	}
	metricsPort := newFakeMetrics()
	logger := &fakeLogger{}

	ctrl := NewController(k8sPort, logger, metricsPort, lbMap, cfg)
	return ctrl, lbAdapter, k8sPort, metricsPort
}

func TestController_SyncUpstreamsForService_ReadyPods(t *testing.T) {
	ctrl, lbAdapter, k8sPort, metricsPort := setupTestController(t)
	ctx := context.Background()

	svc := model.Service{
		Name:      "test-svc",
		Namespace: "default",
		Labels: map[string]string{
			"mlb-loadbalancer-name": "test-lb",
			"mlb-upstream-name":     "test-upstream",
			"mlb-port":              "30080",
		},
		NodePort: 30080,
		Selector: map[string]string{"app": "test"},
	}

	k8sPort.pods = []model.Pod{
		{Name: "pod-1", Ready: true, NodeName: "node1"},
		{Name: "pod-2", Ready: true, NodeName: "node2"},
		{Name: "pod-3", Ready: false, NodeName: "node1"}, // not ready
	}

	err := ctrl.syncUpstreamsForService(ctx, svc)
	require.NoError(t, err)

	lbAdapter.mu.Lock()
	require.Len(t, lbAdapter.syncedUpstream, 1)
	up := lbAdapter.syncedUpstream[0]
	lbAdapter.mu.Unlock()

	assert.Equal(t, "test-upstream", up.Name)
	assert.Equal(t, "test-lb", up.LoadBalancer)
	assert.Len(t, up.Backends, 2)
	assert.Equal(t, "192.168.1.10", up.Backends[0].IP)
	assert.Equal(t, int32(30080), up.Backends[0].Port)
	assert.Equal(t, "192.168.1.20", up.Backends[1].IP)

	metricsPort.mu.Lock()
	assert.Equal(t, float64(2), metricsPort.backendsTotals["test-upstream"])
	metricsPort.mu.Unlock()
}

func TestController_SyncUpstreamsForService_ZeroReadyPods(t *testing.T) {
	ctrl, lbAdapter, k8sPort, metricsPort := setupTestController(t)
	ctx := context.Background()

	svc := model.Service{
		Name:      "test-svc",
		Namespace: "default",
		Labels: map[string]string{
			"mlb-loadbalancer-name": "test-lb",
			"mlb-upstream-name":     "test-upstream",
			"mlb-port":              "30080",
		},
		NodePort: 30080,
		Selector: map[string]string{"app": "test"},
	}

	// All pods unready
	k8sPort.pods = []model.Pod{
		{Name: "pod-1", Ready: false, NodeName: "node1"},
	}

	err := ctrl.syncUpstreamsForService(ctx, svc)
	require.NoError(t, err)

	lbAdapter.mu.Lock()
	require.Len(t, lbAdapter.syncedUpstream, 1)
	up := lbAdapter.syncedUpstream[0]
	lbAdapter.mu.Unlock()

	assert.Equal(t, "test-upstream", up.Name)
	assert.Empty(t, up.Backends, "Expected empty backends to drain LB")

	metricsPort.mu.Lock()
	assert.Equal(t, float64(0), metricsPort.backendsTotals["test-upstream"])
	metricsPort.mu.Unlock()
}

func TestController_ImmediateUpstreamCleanupOnServiceDelete(t *testing.T) {
	ctrl, lbAdapter, k8sPort, _ := setupTestController(t)
	ctx := context.Background()

	svc := model.Service{
		Name:      "test-svc",
		Namespace: "default",
		Labels: map[string]string{
			"mlb-loadbalancer-name": "test-lb",
			"mlb-upstream-name":     "test-upstream",
			"mlb-port":              "30080",
		},
		NodePort: 30080,
		Selector: map[string]string{"app": "test"},
	}

	k8sPort.pods = []model.Pod{
		{Name: "pod-1", Ready: true, NodeName: "node1"},
	}

	key := "default/test-svc"

	// 1. Add event
	ctrl.handleServiceEvent(kubernetes.ServiceEvent{
		Type:    kubernetes.EventTypeAdd,
		Service: svc,
	})

	// Process queue item
	err := ctrl.syncHandler(ctx, key)
	require.NoError(t, err)

	state := ctrl.GetState()
	assert.Contains(t, state.Services, key)
	assert.Contains(t, state.Upstreams, "test-lb/test-upstream")

	// 2. Delete event
	ctrl.handleServiceEvent(kubernetes.ServiceEvent{
		Type:    kubernetes.EventTypeDelete,
		Service: svc,
	})

	// Notice that c.state.Services no longer has the key
	stateAfterDeleteEvent := ctrl.GetState()
	assert.NotContains(t, stateAfterDeleteEvent.Services, key)

	// Now syncHandler runs on key (simulating worker picking up the deletion)
	err = ctrl.syncHandler(ctx, key)
	require.NoError(t, err)

	// Upstream must be removed from c.state.Upstreams
	finalState := ctrl.GetState()
	assert.NotContains(t, finalState.Upstreams, "test-lb/test-upstream")

	// And empty upstream must have been synced to the LB adapter
	lbAdapter.mu.Lock()
	require.NotEmpty(t, lbAdapter.syncedUpstream)
	lastUpstream := lbAdapter.syncedUpstream[len(lbAdapter.syncedUpstream)-1]
	lbAdapter.mu.Unlock()

	assert.Equal(t, "test-upstream", lastUpstream.Name)
	assert.Empty(t, lastUpstream.Backends, "Expected empty backends sent to LB on delete")
}

func TestController_ServiceDeletion_SharedUpstream(t *testing.T) {
	ctrl, lbAdapter, k8sPort, _ := setupTestController(t)
	ctx := context.Background()

	svc1 := model.Service{
		Name:      "svc-1",
		Namespace: "default",
		Labels: map[string]string{
			"mlb-loadbalancer-name": "test-lb",
			"mlb-upstream-name":     "shared-upstream",
			"mlb-port":              "30080",
		},
		NodePort: 30080,
		Selector: map[string]string{"app": "shared"},
	}

	svc2 := model.Service{
		Name:      "svc-2",
		Namespace: "default",
		Labels: map[string]string{
			"mlb-loadbalancer-name": "test-lb",
			"mlb-upstream-name":     "shared-upstream",
			"mlb-port":              "30080",
		},
		NodePort: 30080,
		Selector: map[string]string{"app": "shared"},
	}

	k8sPort.pods = []model.Pod{
		{Name: "pod-1", Ready: true, NodeName: "node1"},
	}

	// Add both services
	ctrl.handleServiceEvent(kubernetes.ServiceEvent{Type: kubernetes.EventTypeAdd, Service: svc1})
	ctrl.handleServiceEvent(kubernetes.ServiceEvent{Type: kubernetes.EventTypeAdd, Service: svc2})
	_ = ctrl.syncHandler(ctx, "default/svc-1")
	_ = ctrl.syncHandler(ctx, "default/svc-2")

	// Delete svc1
	ctrl.handleServiceEvent(kubernetes.ServiceEvent{Type: kubernetes.EventTypeDelete, Service: svc1})
	err := ctrl.syncHandler(ctx, "default/svc-1")
	require.NoError(t, err)

	// Since svc2 still targets shared-upstream, upstream should NOT be purged
	state := ctrl.GetState()
	assert.Contains(t, state.Upstreams, "test-lb/shared-upstream")
	lbAdapter.mu.Lock()
	lastUpstream := lbAdapter.syncedUpstream[len(lbAdapter.syncedUpstream)-1]
	lbAdapter.mu.Unlock()
	assert.NotEmpty(t, lastUpstream.Backends)
}

func TestController_GetState_DeepCopy(t *testing.T) {
	ctrl, _, _, _ := setupTestController(t)

	svc := model.Service{
		Name:      "test-svc",
		Namespace: "default",
	}

	ctrl.stateMutex.Lock()
	ctrl.state.Services["default/test-svc"] = svc
	ctrl.stateMutex.Unlock()

	state1 := ctrl.GetState()
	// Mutate returned map
	state1.Services["default/mutated"] = model.Service{Name: "mutated"}

	state2 := ctrl.GetState()
	assert.NotContains(t, state2.Services, "default/mutated", "GetState must return a deep copy to prevent data races")
}

func TestController_SyncUpstreams_BulkSync(t *testing.T) {
	ctrl, lbAdapter, k8sPort, _ := setupTestController(t)
	ctx := context.Background()

	svc := model.Service{
		Name:      "test-svc",
		Namespace: "default",
		Labels: map[string]string{
			"mlb-loadbalancer-name": "test-lb",
			"mlb-upstream-name":     "bulk-upstream",
			"mlb-port":              "30080",
		},
		NodePort: 30080,
		Selector: map[string]string{"app": "bulk"},
	}

	k8sPort.pods = []model.Pod{
		{Name: "pod-1", Ready: true, NodeName: "node1"},
	}

	ctrl.stateMutex.Lock()
	ctrl.state.Services["default/test-svc"] = svc
	ctrl.stateMutex.Unlock()

	err := ctrl.SyncUpstreams(ctx)
	require.NoError(t, err)

	lbAdapter.mu.Lock()
	defer lbAdapter.mu.Unlock()
	assert.Contains(t, lbAdapter.allUpstreams, "bulk-upstream")
	assert.Len(t, lbAdapter.allUpstreams["bulk-upstream"].Backends, 1)
}
