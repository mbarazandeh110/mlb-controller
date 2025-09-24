// internal/adapters/kubernetes/client_test.go
package kubernetes

import (
	"context"
	"testing"
	"time"

	"mlb-controller/internal/domain/config"
	"mlb-controller/internal/ports/logging"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sclient "k8s.io/client-go/kubernetes/fake"
)

// fakeLogger is a minimal implementation of logging.Logger for testing.
type fakeLogger struct {
	logger *zap.Logger
}

func (f *fakeLogger) Debug(msg string, fields ...logging.Field) {}
func (f *fakeLogger) Info(msg string, fields ...logging.Field)  {}
func (f *fakeLogger) Warn(msg string, fields ...logging.Field)  {}
func (f *fakeLogger) Error(msg string, fields ...logging.Field) {}
func (f *fakeLogger) Fatal(msg string, fields ...logging.Field) {}
func (f *fakeLogger) With(fields ...logging.Field) logging.Logger {
	return f
}
func (f *fakeLogger) Sync() error { return nil }

func TestKubernetesAdapter(t *testing.T) {
	// Create fake client with test data
	fakeClient := k8sclient.NewSimpleClientset(
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "svc1",
				Namespace: "default",
				Labels:    map[string]string{"mlb-loadbalancer-name": "lb1"},
			},
			Spec: corev1.ServiceSpec{
				Type:     corev1.ServiceTypeNodePort,
				Selector: map[string]string{"app": "test"},
				Ports:    []corev1.ServicePort{{NodePort: 30080}},
			},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pod1",
				Namespace: "default",
				Labels:    map[string]string{"app": "test"},
			},
			Spec: corev1.PodSpec{
				NodeName:   "node1",
				Containers: []corev1.Container{{Name: "c1", ReadinessProbe: &corev1.Probe{}}},
			},
			Status: corev1.PodStatus{
				PodIP: "10.0.0.1",
				Phase: corev1.PodRunning,
				Conditions: []corev1.PodCondition{
					{Type: corev1.PodReady, Status: corev1.ConditionTrue},
				},
				ContainerStatuses: []corev1.ContainerStatus{{Name: "c1", Ready: true}},
			},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pod2",
				Namespace: "default",
				Labels:    map[string]string{"app": "test"},
			},
			Spec: corev1.PodSpec{
				NodeName:   "node1",
				Containers: []corev1.Container{{Name: "c1"}}, // No readiness probe
			},
			Status: corev1.PodStatus{
				PodIP: "10.0.0.2",
				Phase: corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{
					{Name: "c1", Ready: true},
				},
			},
		},
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node1",
			},
			Status: corev1.NodeStatus{
				Addresses: []corev1.NodeAddress{
					{Type: corev1.NodeInternalIP, Address: "192.168.1.1"},
					{Type: corev1.NodeExternalIP, Address: "203.0.113.1"},
				},
			},
		},
	)

	// Create test logger
	zapLogger := zaptest.NewLogger(t)
	logger := &fakeLogger{logger: zapLogger}

	// Create adapter
	cfg := config.KubernetesConfig{
		ResyncPeriod:     30 * time.Second,
		KubernetesConfig: "",
	}
	adapter := &KubernetesAdapter{
		client: fakeClient,
		logger: logger,
		config: cfg,
	}

	t.Run("GetPodsForSelector returns matching pods with selector", func(t *testing.T) {
		ctx := context.Background()
		pods, err := adapter.GetPodsForSelector(ctx, "default", map[string]string{"app": "test"})
		assert.NoError(t, err)
		assert.Len(t, pods, 2)
		assert.Equal(t, "pod1", pods[0].Name)
		assert.Equal(t, "10.0.0.1", pods[0].IP)
		assert.True(t, pods[0].Ready)
		assert.Equal(t, "pod2", pods[1].Name)
		assert.Equal(t, "10.0.0.2", pods[1].IP)
		assert.True(t, pods[1].Ready)
	})

	t.Run("GetPodsForSelector returns all pods with empty selector", func(t *testing.T) {
		ctx := context.Background()
		pods, err := adapter.GetPodsForSelector(ctx, "default", map[string]string{})
		assert.NoError(t, err)
		assert.Len(t, pods, 2)
	})

	t.Run("GetNodeByName returns node", func(t *testing.T) {
		ctx := context.Background()
		node, err := adapter.GetNodeByName(ctx, "node1")
		assert.NoError(t, err)
		assert.Equal(t, "node1", node.Name)
		assert.Equal(t, "192.168.1.1", node.InternalIP)
		assert.Equal(t, "203.0.113.1", node.ExternalIP)
	})

	t.Run("isPodReady returns true for pod with readiness probe", func(t *testing.T) {
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

	t.Run("isPodReady returns false for pod with failed readiness probe", func(t *testing.T) {
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

	t.Run("isPodReady returns true for pod without probe and all containers ready", func(t *testing.T) {
		pod := &corev1.Pod{
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{{Name: "c1"}},
			},
			Status: corev1.PodStatus{
				Phase:             corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{{Name: "c1", Ready: true}},
			},
		}
		assert.True(t, adapter.isPodReady(pod))
	})

	t.Run("isPodReady returns false for pod without probe and not all containers ready", func(t *testing.T) {
		pod := &corev1.Pod{
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{{Name: "c1"}},
			},
			Status: corev1.PodStatus{
				Phase:             corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{{Name: "c1", Ready: false}},
			},
		}
		assert.False(t, adapter.isPodReady(pod))
	})

	t.Run("isPodReady returns false for non-running pod without probe", func(t *testing.T) {
		pod := &corev1.Pod{
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{{Name: "c1"}},
			},
			Status: corev1.PodStatus{
				Phase:             corev1.PodPending,
				ContainerStatuses: []corev1.ContainerStatus{{Name: "c1", Ready: true}},
			},
		}
		assert.False(t, adapter.isPodReady(pod))
	})
}
