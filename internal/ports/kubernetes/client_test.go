package kubernetes

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"mlb-controller/internal/domain/model"

	"github.com/stretchr/testify/assert"
)

// fakeKubernetes is a fake implementation of KubernetesPort for testing.
type fakeKubernetes struct {
	services map[string]model.Service
	pods     map[string]model.Pod
	nodes    map[string]model.Node
	mu       sync.Mutex
}

func (f *fakeKubernetes) WatchServices(ctx context.Context, labels map[string]string, events chan<- ServiceEvent) error {
	f.mu.Lock()
	svcs := make([]model.Service, 0, len(f.services))
	for _, svc := range f.services {
		svcs = append(svcs, svc)
	}
	f.mu.Unlock()

	// Simulate sending service events
	for _, svc := range svcs {
		select {
		case events <- ServiceEvent{Type: EventTypeAdd, Service: svc}:
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	return nil
}

func (f *fakeKubernetes) WatchPods(ctx context.Context, events chan<- PodEvent) error {
	f.mu.Lock()
	pods := make([]model.Pod, 0, len(f.pods))
	for _, pod := range f.pods {
		pods = append(pods, pod)
	}
	f.mu.Unlock()

	// Simulate sending pod events
	for _, pod := range pods {
		select {
		case events <- PodEvent{Type: EventTypeAdd, Pod: pod}:
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	return nil
}

func (f *fakeKubernetes) WatchNodes(ctx context.Context, events chan<- NodeEvent) error {
	f.mu.Lock()
	nodes := make([]model.Node, 0, len(f.nodes))
	for _, node := range f.nodes {
		nodes = append(nodes, node)
	}
	f.mu.Unlock()

	// Simulate sending node events
	for _, node := range nodes {
		select {
		case events <- NodeEvent{Type: EventTypeAdd, Node: node}:
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	return nil
}

func (f *fakeKubernetes) GetPodsForSelector(ctx context.Context, namespace string, selector map[string]string) ([]model.Pod, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	var pods []model.Pod
	for _, pod := range f.pods {
		if pod.Namespace == namespace {
			// Simplified selector matching for testing
			pods = append(pods, pod)
		}
	}
	return pods, nil
}

func (f *fakeKubernetes) GetNodeByName(ctx context.Context, name string) (model.Node, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	node, exists := f.nodes[name]
	if !exists {
		return model.Node{}, nil // Simulate not found
	}
	return node, nil
}

func (f *fakeKubernetes) Start(ctx context.Context) error {
	return nil
}

func (f *fakeKubernetes) GetService(namespace, name string) (*model.Service, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	key := fmt.Sprintf("%s/%s", namespace, name)
	svc, exists := f.services[key]
	if !exists {
		return nil, fmt.Errorf("service not found: %s", key)
	}
	return &svc, nil
}

func (f *fakeKubernetes) ListServices() ([]model.Service, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var result []model.Service
	for _, svc := range f.services {
		result = append(result, svc)
	}
	return result, nil
}

func (f *fakeKubernetes) HasSynced() bool {
	return true
}

func TestKubernetesPort(t *testing.T) {
	fake := &fakeKubernetes{
		services: map[string]model.Service{
			"ns/svc1": {
				Name:      "svc1",
				Namespace: "ns",
				Labels:    map[string]string{"mlb-loadbalancer-name": "lb1"},
			},
		},
		pods: map[string]model.Pod{
			"ns/pod1": {
				Name:      "pod1",
				Namespace: "ns",
				IP:        "10.0.0.1",
				Ready:     true,
			},
		},
		nodes: map[string]model.Node{
			"node1": {
				Name:       "node1",
				InternalIP: "192.168.1.1",
			},
		},
	}

	t.Run("WatchServices sends events", func(t *testing.T) {
		events := make(chan ServiceEvent, 1)
		err := fake.WatchServices(context.Background(), map[string]string{"mlb-loadbalancer-name": "lb1"}, events)
		assert.NoError(t, err)

		select {
		case evt := <-events:
			assert.Equal(t, EventTypeAdd, evt.Type)
			assert.Equal(t, "svc1", evt.Service.Name)
		default:
			t.Fatal("no service event received")
		}
	})

	t.Run("WatchPods sends events", func(t *testing.T) {
		events := make(chan PodEvent, 1)
		err := fake.WatchPods(context.Background(), events)
		assert.NoError(t, err)

		select {
		case evt := <-events:
			assert.Equal(t, EventTypeAdd, evt.Type)
			assert.Equal(t, "pod1", evt.Pod.Name)
		default:
			t.Fatal("no pod event received")
		}
	})

	t.Run("WatchNodes sends events", func(t *testing.T) {
		events := make(chan NodeEvent, 1)
		err := fake.WatchNodes(context.Background(), events)
		assert.NoError(t, err)

		select {
		case evt := <-events:
			assert.Equal(t, EventTypeAdd, evt.Type)
			assert.Equal(t, "node1", evt.Node.Name)
		default:
			t.Fatal("no node event received")
		}
	})

	t.Run("GetPodsForSelector returns pods", func(t *testing.T) {
		ctx := context.Background()
		pods, err := fake.GetPodsForSelector(ctx, "ns", map[string]string{})
		assert.NoError(t, err)
		assert.Len(t, pods, 1)
		assert.Equal(t, "pod1", pods[0].Name)
	})

	t.Run("GetNodeByName returns node", func(t *testing.T) {
		ctx := context.Background()
		node, err := fake.GetNodeByName(ctx, "node1")
		assert.NoError(t, err)
		assert.Equal(t, "node1", node.Name)
		assert.Equal(t, "192.168.1.1", node.InternalIP)
	})
}
