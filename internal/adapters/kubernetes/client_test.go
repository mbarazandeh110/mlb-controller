// internal/adapters/kubernetes/client_test.go
package kubernetes

import (
	"context"
	"testing"
	"time"

	"mlb-controller/internal/domain/config"
	port_k8s "mlb-controller/internal/ports/kubernetes"
	"mlb-controller/internal/ports/logging"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	k8sclient "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/cache"
)

// fakeLogger is a minimal implementation of logging.Logger for testing.
type fakeLogger struct{}

func (f *fakeLogger) Debug(msg string, fields ...logging.Field)   {}
func (f *fakeLogger) Info(msg string, fields ...logging.Field)    {}
func (f *fakeLogger) Warn(msg string, fields ...logging.Field)    {}
func (f *fakeLogger) Error(msg string, fields ...logging.Field)   {}
func (f *fakeLogger) Fatal(msg string, fields ...logging.Field)   {}
func (f *fakeLogger) With(fields ...logging.Field) logging.Logger { return f }
func (f *fakeLogger) Sync() error                                 { return nil }

func TestKubernetesAdapter_IsMLBService(t *testing.T) {
	adapter := &KubernetesAdapter{}

	tests := []struct {
		name     string
		svc      *corev1.Service
		expected bool
	}{
		{
			name: "Valid NodePort with all labels",
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"mlb-loadbalancer-name": "lb1",
						"mlb-upstream-name":     "up1",
						"mlb-port":              "30080",
					},
				},
				Spec: corev1.ServiceSpec{Type: corev1.ServiceTypeNodePort},
			},
			expected: true,
		},
		{
			name: "Valid LoadBalancer with all labels",
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"mlb-loadbalancer-name": "lb1",
						"mlb-upstream-name":     "up1",
						"mlb-port":              "30080",
					},
				},
				Spec: corev1.ServiceSpec{Type: corev1.ServiceTypeLoadBalancer},
			},
			expected: true,
		},
		{
			name: "Invalid ServiceType ClusterIP",
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"mlb-loadbalancer-name": "lb1",
						"mlb-upstream-name":     "up1",
						"mlb-port":              "30080",
					},
				},
				Spec: corev1.ServiceSpec{Type: corev1.ServiceTypeClusterIP},
			},
			expected: false,
		},
		{
			name: "Missing mlb-loadbalancer-name label",
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"mlb-upstream-name": "up1",
						"mlb-port":          "30080",
					},
				},
				Spec: corev1.ServiceSpec{Type: corev1.ServiceTypeNodePort},
			},
			expected: false,
		},
		{
			name: "Missing mlb-upstream-name label",
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"mlb-loadbalancer-name": "lb1",
						"mlb-port":              "30080",
					},
				},
				Spec: corev1.ServiceSpec{Type: corev1.ServiceTypeNodePort},
			},
			expected: false,
		},
		{
			name: "Missing mlb-port label",
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"mlb-loadbalancer-name": "lb1",
						"mlb-upstream-name":     "up1",
					},
				},
				Spec: corev1.ServiceSpec{Type: corev1.ServiceTypeNodePort},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, adapter.isMLBService(tt.svc))
		})
	}
}

func TestKubernetesAdapter_GetNodePort(t *testing.T) {
	adapter := &KubernetesAdapter{}

	t.Run("Port from label takes precedence", func(t *testing.T) {
		svc := &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{"mlb-port": "31000"},
			},
			Spec: corev1.ServiceSpec{
				Type:  corev1.ServiceTypeNodePort,
				Ports: []corev1.ServicePort{{NodePort: 32000}},
			},
		}
		assert.Equal(t, int32(31000), adapter.getNodePort(svc))
	})

	t.Run("Fallback to Spec.Ports[0].NodePort when label missing", func(t *testing.T) {
		svc := &corev1.Service{
			Spec: corev1.ServiceSpec{
				Type:  corev1.ServiceTypeNodePort,
				Ports: []corev1.ServicePort{{NodePort: 32000}},
			},
		}
		assert.Equal(t, int32(32000), adapter.getNodePort(svc))
	})

	t.Run("Returns 0 when no port is available", func(t *testing.T) {
		svc := &corev1.Service{
			Spec: corev1.ServiceSpec{
				Type: corev1.ServiceTypeClusterIP,
			},
		}
		assert.Equal(t, int32(0), adapter.getNodePort(svc))
	})
}

func TestKubernetesAdapter_IsPodReady(t *testing.T) {
	adapter := &KubernetesAdapter{}

	t.Run("Ready pod with readiness probe", func(t *testing.T) {
		pod := &corev1.Pod{
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{{ReadinessProbe: &corev1.Probe{}}},
			},
			Status: corev1.PodStatus{
				Conditions: []corev1.PodCondition{
					{Type: corev1.PodReady, Status: corev1.ConditionTrue},
				},
			},
		}
		assert.True(t, adapter.isPodReady(pod))
	})

	t.Run("Unready pod with failed readiness probe", func(t *testing.T) {
		pod := &corev1.Pod{
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{{ReadinessProbe: &corev1.Probe{}}},
			},
			Status: corev1.PodStatus{
				Conditions: []corev1.PodCondition{
					{Type: corev1.PodReady, Status: corev1.ConditionFalse},
				},
			},
		}
		assert.False(t, adapter.isPodReady(pod))
	})

	t.Run("Pod without probe - all containers ready", func(t *testing.T) {
		pod := &corev1.Pod{
			Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "c1"}}},
			Status: corev1.PodStatus{
				Phase:             corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{{Name: "c1", Ready: true}},
			},
		}
		assert.True(t, adapter.isPodReady(pod))
	})

	t.Run("Pod without probe - container not ready", func(t *testing.T) {
		pod := &corev1.Pod{
			Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "c1"}}},
			Status: corev1.PodStatus{
				Phase:             corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{{Name: "c1", Ready: false}},
			},
		}
		assert.False(t, adapter.isPodReady(pod))
	})

	t.Run("Pod without probe - non-running phase", func(t *testing.T) {
		pod := &corev1.Pod{
			Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "c1"}}},
			Status: corev1.PodStatus{
				Phase:             corev1.PodPending,
				ContainerStatuses: []corev1.ContainerStatus{{Name: "c1", Ready: true}},
			},
		}
		assert.False(t, adapter.isPodReady(pod))
	})
}

func setupTestKubernetesAdapter(t *testing.T) (*KubernetesAdapter, *k8sclient.Clientset, context.CancelFunc) {
	fakeClient := k8sclient.NewSimpleClientset()
	informerFactory := informers.NewSharedInformerFactory(fakeClient, 0)
	logger := &fakeLogger{}

	adapter := &KubernetesAdapter{
		client:          fakeClient,
		logger:          logger,
		config:          config.KubernetesConfig{ResyncPeriod: 10 * time.Minute},
		informerFactory: informerFactory,
	}

	_, cancel := context.WithCancel(context.Background())
	return adapter, fakeClient, cancel
}

func TestKubernetesAdapter_WatchServices(t *testing.T) {
	adapter, fakeClient, cancel := setupTestKubernetesAdapter(t)
	defer cancel()

	events := make(chan port_k8s.ServiceEvent, 20)
	ctx, watchCancel := context.WithCancel(context.Background())
	defer watchCancel()

	go func() {
		_ = adapter.WatchServices(ctx, nil, events)
	}()

	// 1. Create MLB service -> EventTypeAdd
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-svc",
			Namespace: "default",
			Labels: map[string]string{
				"mlb-loadbalancer-name": "lb1",
				"mlb-upstream-name":     "up1",
				"mlb-port":              "30080",
			},
		},
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceTypeNodePort,
			Selector: map[string]string{"app": "demo"},
			Ports:    []corev1.ServicePort{{NodePort: 30080}},
		},
	}

	_, err := fakeClient.CoreV1().Services("default").Create(context.Background(), svc, metav1.CreateOptions{})
	require.NoError(t, err)

	select {
	case ev := <-events:
		assert.Equal(t, port_k8s.EventTypeAdd, ev.Type)
		assert.Equal(t, "test-svc", ev.Service.Name)
		assert.Equal(t, int32(30080), ev.Service.NodePort)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for Service Add event")
	}

	// 2. Update service to non-MLB (remove upstream label) -> EventTypeDelete (declassification)
	svcUpdated := svc.DeepCopy()
	delete(svcUpdated.Labels, "mlb-upstream-name")
	_, err = fakeClient.CoreV1().Services("default").Update(context.Background(), svcUpdated, metav1.UpdateOptions{})
	require.NoError(t, err)

	select {
	case ev := <-events:
		assert.Equal(t, port_k8s.EventTypeDelete, ev.Type)
		assert.Equal(t, "test-svc", ev.Service.Name)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for Service Delete (declassification) event")
	}

	// 3. Update service back to MLB with modified port -> EventTypeUpdate
	svcReclassified := svc.DeepCopy()
	svcReclassified.Labels["mlb-port"] = "30090"
	svcReclassified.Spec.Ports[0].NodePort = 30090
	_, err = fakeClient.CoreV1().Services("default").Update(context.Background(), svcReclassified, metav1.UpdateOptions{})
	require.NoError(t, err)

	select {
	case ev := <-events:
		assert.Equal(t, port_k8s.EventTypeUpdate, ev.Type)
		assert.Equal(t, int32(30090), ev.Service.NodePort)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for Service Update event")
	}

	// 4. Delete service -> EventTypeDelete
	err = fakeClient.CoreV1().Services("default").Delete(context.Background(), "test-svc", metav1.DeleteOptions{})
	require.NoError(t, err)

	select {
	case ev := <-events:
		assert.Equal(t, port_k8s.EventTypeDelete, ev.Type)
		assert.Equal(t, "test-svc", ev.Service.Name)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for Service Delete event")
	}

	// 5. Test tombstone handling in DeleteFunc directly
	tombstone := cache.DeletedFinalStateUnknown{
		Key: "default/tomb-svc",
		Obj: svc,
	}
	// Verify unwrapping works cleanly
	var obj interface{} = tombstone
	if tObj, ok := obj.(cache.DeletedFinalStateUnknown); ok {
		obj = tObj.Obj
	}
	s, ok := obj.(*corev1.Service)
	assert.True(t, ok)
	assert.Equal(t, "test-svc", s.Name)
}

func TestKubernetesAdapter_WatchPods(t *testing.T) {
	adapter, fakeClient, cancel := setupTestKubernetesAdapter(t)
	defer cancel()

	events := make(chan port_k8s.PodEvent, 20)
	ctx, watchCancel := context.WithCancel(context.Background())
	defer watchCancel()

	go func() {
		_ = adapter.WatchPods(ctx, events)
	}()

	// 1. Create Pod -> EventTypeAdd
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
			Labels:    map[string]string{"app": "demo"},
		},
		Spec: corev1.PodSpec{
			NodeName:   "node-1",
			Containers: []corev1.Container{{Name: "c1"}},
		},
		Status: corev1.PodStatus{
			Phase:             corev1.PodRunning,
			PodIP:             "10.244.0.5",
			ContainerStatuses: []corev1.ContainerStatus{{Name: "c1", Ready: true}},
		},
	}

	_, err := fakeClient.CoreV1().Pods("default").Create(context.Background(), pod, metav1.CreateOptions{})
	require.NoError(t, err)

	select {
	case ev := <-events:
		assert.Equal(t, port_k8s.EventTypeAdd, ev.Type)
		assert.Equal(t, "test-pod", ev.Pod.Name)
		assert.Equal(t, "10.244.0.5", ev.Pod.IP)
		assert.True(t, ev.Pod.Ready)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for Pod Add event")
	}

	// 2. Update Pod (unready) -> EventTypeUpdate
	podUpdated := pod.DeepCopy()
	podUpdated.Status.ContainerStatuses[0].Ready = false
	_, err = fakeClient.CoreV1().Pods("default").UpdateStatus(context.Background(), podUpdated, metav1.UpdateOptions{})
	require.NoError(t, err)

	select {
	case ev := <-events:
		assert.Equal(t, port_k8s.EventTypeUpdate, ev.Type)
		assert.Equal(t, "test-pod", ev.Pod.Name)
		assert.False(t, ev.Pod.Ready)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for Pod Update event")
	}

	// 3. Delete Pod -> EventTypeDelete
	err = fakeClient.CoreV1().Pods("default").Delete(context.Background(), "test-pod", metav1.DeleteOptions{})
	require.NoError(t, err)

	select {
	case ev := <-events:
		assert.Equal(t, port_k8s.EventTypeDelete, ev.Type)
		assert.Equal(t, "test-pod", ev.Pod.Name)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for Pod Delete event")
	}

	// 4. Delete with tombstone
	tombstone := cache.DeletedFinalStateUnknown{
		Key: "default/tomb-pod",
		Obj: pod,
	}
	var obj interface{} = tombstone
	if tObj, ok := obj.(cache.DeletedFinalStateUnknown); ok {
		obj = tObj.Obj
	}
	p, ok := obj.(*corev1.Pod)
	assert.True(t, ok)
	assert.Equal(t, "test-pod", p.Name)
}

func TestKubernetesAdapter_WatchNodes(t *testing.T) {
	adapter, fakeClient, cancel := setupTestKubernetesAdapter(t)
	defer cancel()

	events := make(chan port_k8s.NodeEvent, 20)
	ctx, watchCancel := context.WithCancel(context.Background())
	defer watchCancel()

	go func() {
		_ = adapter.WatchNodes(ctx, events)
	}()

	// 1. Create Node -> EventTypeAdd
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
		},
		Status: corev1.NodeStatus{
			Addresses: []corev1.NodeAddress{
				{Type: corev1.NodeInternalIP, Address: "192.168.1.50"},
				{Type: corev1.NodeExternalIP, Address: "203.0.113.50"},
			},
		},
	}

	_, err := fakeClient.CoreV1().Nodes().Create(context.Background(), node, metav1.CreateOptions{})
	require.NoError(t, err)

	select {
	case ev := <-events:
		assert.Equal(t, port_k8s.EventTypeAdd, ev.Type)
		assert.Equal(t, "test-node", ev.Node.Name)
		assert.Equal(t, "192.168.1.50", ev.Node.InternalIP)
		assert.Equal(t, "203.0.113.50", ev.Node.ExternalIP)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for Node Add event")
	}

	// 2. Update Node -> EventTypeUpdate
	nodeUpdated := node.DeepCopy()
	nodeUpdated.Status.Addresses[0].Address = "192.168.1.51"
	_, err = fakeClient.CoreV1().Nodes().UpdateStatus(context.Background(), nodeUpdated, metav1.UpdateOptions{})
	require.NoError(t, err)

	select {
	case ev := <-events:
		assert.Equal(t, port_k8s.EventTypeUpdate, ev.Type)
		assert.Equal(t, "test-node", ev.Node.Name)
		assert.Equal(t, "192.168.1.51", ev.Node.InternalIP)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for Node Update event")
	}

	// 3. Delete Node -> EventTypeDelete
	err = fakeClient.CoreV1().Nodes().Delete(context.Background(), "test-node", metav1.DeleteOptions{})
	require.NoError(t, err)

	select {
	case ev := <-events:
		assert.Equal(t, port_k8s.EventTypeDelete, ev.Type)
		assert.Equal(t, "test-node", ev.Node.Name)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for Node Delete event")
	}

	// 4. Delete with tombstone
	tombstone := cache.DeletedFinalStateUnknown{
		Key: "test-node",
		Obj: node,
	}
	var obj interface{} = tombstone
	if tObj, ok := obj.(cache.DeletedFinalStateUnknown); ok {
		obj = tObj.Obj
	}
	n, ok := obj.(*corev1.Node)
	assert.True(t, ok)
	assert.Equal(t, "test-node", n.Name)
}

func TestKubernetesAdapter_GetPodsForSelector(t *testing.T) {
	fakeClient := k8sclient.NewSimpleClientset(
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pod-a",
				Namespace: "default",
				Labels:    map[string]string{"app": "foo", "env": "prod"},
			},
			Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "c1"}}},
			Status: corev1.PodStatus{
				Phase:             corev1.PodRunning,
				PodIP:             "10.0.0.1",
				ContainerStatuses: []corev1.ContainerStatus{{Name: "c1", Ready: true}},
			},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pod-b",
				Namespace: "default",
				Labels:    map[string]string{"app": "bar"},
			},
			Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "c1"}}},
			Status: corev1.PodStatus{
				Phase:             corev1.PodRunning,
				PodIP:             "10.0.0.2",
				ContainerStatuses: []corev1.ContainerStatus{{Name: "c1", Ready: true}},
			},
		},
	)

	informerFactory := informers.NewSharedInformerFactory(fakeClient, 0)
	podInformer := informerFactory.Core().V1().Pods().Informer()

	stopCh := make(chan struct{})
	defer close(stopCh)
	informerFactory.Start(stopCh)
	require.True(t, cache.WaitForCacheSync(stopCh, podInformer.HasSynced))

	adapter := &KubernetesAdapter{
		client:          fakeClient,
		logger:          &fakeLogger{},
		informerFactory: informerFactory,
	}

	ctx := context.Background()

	t.Run("Matching selector", func(t *testing.T) {
		pods, err := adapter.GetPodsForSelector(ctx, "default", map[string]string{"app": "foo"})
		assert.NoError(t, err)
		assert.Len(t, pods, 1)
		assert.Equal(t, "pod-a", pods[0].Name)
	})

	t.Run("Empty selector returns empty slice to prevent accidental wildcard matching", func(t *testing.T) {
		pods, err := adapter.GetPodsForSelector(ctx, "default", map[string]string{})
		assert.NoError(t, err)
		assert.Empty(t, pods)
	})

	t.Run("Non-matching selector returns empty slice", func(t *testing.T) {
		pods, err := adapter.GetPodsForSelector(ctx, "default", map[string]string{"app": "nonexistent"})
		assert.NoError(t, err)
		assert.Empty(t, pods)
	})
}

func TestKubernetesAdapter_GetNodeByName(t *testing.T) {
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "worker-1"},
		Status: corev1.NodeStatus{
			Addresses: []corev1.NodeAddress{
				{Type: corev1.NodeInternalIP, Address: "192.168.1.10"},
				{Type: corev1.NodeExternalIP, Address: "203.0.113.10"},
			},
		},
	}

	fakeClient := k8sclient.NewSimpleClientset(node)
	informerFactory := informers.NewSharedInformerFactory(fakeClient, 0)
	nodeInformer := informerFactory.Core().V1().Nodes().Informer()

	stopCh := make(chan struct{})
	defer close(stopCh)
	informerFactory.Start(stopCh)
	require.True(t, cache.WaitForCacheSync(stopCh, nodeInformer.HasSynced))

	adapter := &KubernetesAdapter{
		client:          fakeClient,
		logger:          &fakeLogger{},
		informerFactory: informerFactory,
	}

	ctx := context.Background()

	t.Run("Existing node from cache", func(t *testing.T) {
		res, err := adapter.GetNodeByName(ctx, "worker-1")
		assert.NoError(t, err)
		assert.Equal(t, "worker-1", res.Name)
		assert.Equal(t, "192.168.1.10", res.InternalIP)
		assert.Equal(t, "203.0.113.10", res.ExternalIP)
	})

	t.Run("Non-existent node returns error", func(t *testing.T) {
		_, err := adapter.GetNodeByName(ctx, "non-existent")
		assert.Error(t, err)
	})

	t.Run("Fallback to API when not in informer cache", func(t *testing.T) {
		// Create a separate un-synced adapter to force cache miss
		emptyInformerFactory := informers.NewSharedInformerFactory(k8sclient.NewSimpleClientset(), 0)
		fallbackAdapter := &KubernetesAdapter{
			client:          fakeClient, // client has worker-1
			logger:          &fakeLogger{},
			informerFactory: emptyInformerFactory,
		}

		res, err := fallbackAdapter.GetNodeByName(ctx, "worker-1")
		assert.NoError(t, err)
		assert.Equal(t, "worker-1", res.Name)
		assert.Equal(t, "192.168.1.10", res.InternalIP)
	})
}

func TestKubernetesAdapter_HasSynced(t *testing.T) {
	fakeClient := k8sclient.NewSimpleClientset()
	informerFactory := informers.NewSharedInformerFactory(fakeClient, 0)
	// Touch informers to instantiate them
	_ = informerFactory.Core().V1().Services().Informer()
	_ = informerFactory.Core().V1().Pods().Informer()
	_ = informerFactory.Core().V1().Nodes().Informer()

	adapter := &KubernetesAdapter{
		client:          fakeClient,
		logger:          &fakeLogger{},
		informerFactory: informerFactory,
	}

	assert.False(t, adapter.HasSynced(), "before informer start, HasSynced should be false")

	stopCh := make(chan struct{})
	defer close(stopCh)
	informerFactory.Start(stopCh)
	informerFactory.WaitForCacheSync(stopCh)

	assert.True(t, adapter.HasSynced(), "after informer cache sync, HasSynced should be true")

	nilAdapter := &KubernetesAdapter{informerFactory: nil}
	assert.False(t, nilAdapter.HasSynced(), "nil informerFactory should return false")
}
