package kubernetes

import (
	"context"
	"sync"
	"testing"
	"time"

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
	defer f.mu.Unlock()

	// Simulate sending one service event
	for _, svc := range f.services {
		select {
		case events <- ServiceEvent{Type: EventTypeAdd, Service: svc}:
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	// Block until context is canceled
	<-ctx.Done()
	return ctx.Err()
}

func (f *fakeKubernetes) WatchPods(ctx context.Context, events chan<- PodEvent) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	// Simulate sending one pod event
	for _, pod := range f.pods {
		select {
		case events <- PodEvent{Type: EventTypeAdd, Pod: pod}:
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	<-ctx.Done()
	return ctx.Err()
}

func (f *fakeKubernetes) WatchNodes(ctx context.Context, events chan<- NodeEvent) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	// Simulate sending one node event
	for _, node := range f.nodes {
		select {
		case events <- NodeEvent{Type: EventTypeAdd, Node: node}:
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	<-ctx.Done()
	return ctx.Err()
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
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		events := make(chan ServiceEvent, 1)
		go func() {
			err := fake.WatchServices(ctx, map[string]string{"mlb-loadbalancer-name": "lb1"}, events)
			assert.ErrorIs(t, err, ctx.Err())
		}()

		select {
		case evt := <-events:
			assert.Equal(t, EventTypeAdd, evt.Type)
			assert.Equal(t, "svc1", evt.Service.Name)
		case <-time.After(50 * time.Millisecond):
			t.Fatal("no service event received")
		}
	})

	t.Run("WatchPods sends events", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		events := make(chan PodEvent, 1)
		go func() {
			err := fake.WatchPods(ctx, events)
			assert.ErrorIs(t, err, ctx.Err())
		}()

		select {
		case evt := <-events:
			assert.Equal(t, EventTypeAdd, evt.Type)
			assert.Equal(t, "pod1", evt.Pod.Name)
		case <-time.After(50 * time.Millisecond):
			t.Fatal("no pod event received")
		}
	})

	t.Run("WatchNodes sends events", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		events := make(chan NodeEvent, 1)
		go func() {
			err := fake.WatchNodes(ctx, events)
			assert.ErrorIs(t, err, ctx.Err())
		}()

		select {
		case evt := <-events:
			assert.Equal(t, EventTypeAdd, evt.Type)
			assert.Equal(t, "node1", evt.Node.Name)
		case <-time.After(50 * time.Millisecond):
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
