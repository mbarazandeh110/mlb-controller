// internal/adapters/kubernetes/client.go
package kubernetes

import (
	"context"
	"fmt"
	"strconv"

	"mlb-controller/internal/domain/config"
	"mlb-controller/internal/domain/model"
	"mlb-controller/internal/ports/kubernetes"
	"mlb-controller/internal/ports/logging"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/informers"
	k8sclient "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
)

// KubernetesAdapter implements KubernetesPort using client-go.
type KubernetesAdapter struct {
	client          k8sclient.Interface
	logger          logging.Logger
	config          config.KubernetesConfig
	informerFactory informers.SharedInformerFactory
	started         bool // Flag to ensure Start is called only once
}

// NewKubernetesAdapter creates a new KubernetesAdapter instance.
func NewKubernetesAdapter(k8sCfg config.KubernetesConfig, logger logging.Logger) (*KubernetesAdapter, error) {
	// Build Kubernetes client from config
	kubeConfig, err := clientcmd.BuildConfigFromFlags("", k8sCfg.KubernetesConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to build kubernetes config: %w", err)
	}
	client, err := k8sclient.NewForConfig(kubeConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	// Create informer factory with resync period for periodic sync (aligns with requirement 7)
	informerFactory := informers.NewSharedInformerFactory(client, k8sCfg.ResyncPeriod)

	return &KubernetesAdapter{
		client:          client,
		logger:          logger,
		config:          k8sCfg,
		informerFactory: informerFactory,
		started:         false,
	}, nil
}

// startInformers starts the informer factory if not already started.
func (k *KubernetesAdapter) startInformers(ctx context.Context) {
	if !k.started {
		go k.informerFactory.Start(ctx.Done())
		k.started = true
		k.logger.Info("Kubernetes informers started")
	}
}

// WatchServices watches Kubernetes Services and sends events for those with required MLB labels and types.
func (k *KubernetesAdapter) WatchServices(ctx context.Context, labels map[string]string, events chan<- kubernetes.ServiceEvent) error {
	informer := k.informerFactory.Core().V1().Services().Informer()
	informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			svc, ok := obj.(*corev1.Service)
			if !ok {
				k.logger.Error("Invalid service object in add event")
				return
			}
			if k.isMLBService(svc) {
				k.logger.Debug("Processing add event for MLB service", logging.Field{Key: "service", Value: svc.Name}, logging.Field{Key: "namespace", Value: svc.Namespace})
				events <- kubernetes.ServiceEvent{
					Type: kubernetes.EventTypeAdd,
					Service: model.Service{
						Name:        svc.Name,
						Namespace:   svc.Namespace,
						Labels:      svc.Labels,
						Selector:    svc.Spec.Selector,
						ServiceType: string(svc.Spec.Type),
						NodePort:    k.getNodePort(svc),
					},
				}
			}
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			svc, ok := newObj.(*corev1.Service)
			if !ok {
				k.logger.Error("Invalid service object in update event")
				return
			}
			if k.isMLBService(svc) {
				k.logger.Debug("Processing update event for MLB service", logging.Field{Key: "service", Value: svc.Name}, logging.Field{Key: "namespace", Value: svc.Namespace})
				events <- kubernetes.ServiceEvent{
					Type: kubernetes.EventTypeUpdate,
					Service: model.Service{
						Name:        svc.Name,
						Namespace:   svc.Namespace,
						Labels:      svc.Labels,
						Selector:    svc.Spec.Selector,
						ServiceType: string(svc.Spec.Type),
						NodePort:    k.getNodePort(svc),
					},
				}
			}
		},
		DeleteFunc: func(obj interface{}) {
			svc, ok := obj.(*corev1.Service)
			if !ok {
				k.logger.Error("Invalid service object in delete event")
				return
			}
			if k.isMLBService(svc) {
				k.logger.Debug("Processing delete event for MLB service", logging.Field{Key: "service", Value: svc.Name}, logging.Field{Key: "namespace", Value: svc.Namespace})
				events <- kubernetes.ServiceEvent{
					Type: kubernetes.EventTypeDelete,
					Service: model.Service{
						Name:        svc.Name,
						Namespace:   svc.Namespace,
						Labels:      svc.Labels,
						Selector:    svc.Spec.Selector,
						ServiceType: string(svc.Spec.Type),
						NodePort:    k.getNodePort(svc),
					},
				}
			}
		},
	})

	k.startInformers(ctx)
	if !cache.WaitForCacheSync(ctx.Done(), informer.HasSynced) {
		return fmt.Errorf("failed to sync service informer")
	}

	<-ctx.Done()
	k.logger.Info("WatchServices stopped")
	return ctx.Err()
}

// WatchPods watches all Kubernetes Pods and sends events (requirement 3: monitor pod add/delete/readiness changes).
func (k *KubernetesAdapter) WatchPods(ctx context.Context, events chan<- kubernetes.PodEvent) error {
	informer := k.informerFactory.Core().V1().Pods().Informer()
	informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			pod, ok := obj.(*corev1.Pod)
			if !ok {
				k.logger.Error("Invalid pod object in add event")
				return
			}
			k.logger.Debug("Processing add event for pod", logging.Field{Key: "pod", Value: pod.Name}, logging.Field{Key: "namespace", Value: pod.Namespace})
			events <- kubernetes.PodEvent{
				Type: kubernetes.EventTypeAdd,
				Pod: model.Pod{
					Name:      pod.Name,
					Namespace: pod.Namespace,
					IP:        pod.Status.PodIP,
					Ready:     k.isPodReady(pod),
					NodeName:  pod.Spec.NodeName,
					Status:    string(pod.Status.Phase),
					Labels:    pod.Labels,
				},
			}
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			pod, ok := newObj.(*corev1.Pod)
			if !ok {
				k.logger.Error("Invalid pod object in update event")
				return
			}
			k.logger.Debug("Processing update event for pod", logging.Field{Key: "pod", Value: pod.Name}, logging.Field{Key: "namespace", Value: pod.Namespace})
			events <- kubernetes.PodEvent{
				Type: kubernetes.EventTypeUpdate,
				Pod: model.Pod{
					Name:      pod.Name,
					Namespace: pod.Namespace,
					IP:        pod.Status.PodIP,
					Ready:     k.isPodReady(pod),
					NodeName:  pod.Spec.NodeName,
					Status:    string(pod.Status.Phase),
					Labels:    pod.Labels,
				},
			}
		},
		DeleteFunc: func(obj interface{}) {
			pod, ok := obj.(*corev1.Pod)
			if !ok {
				k.logger.Error("Invalid pod object in delete event")
				return
			}
			k.logger.Debug("Processing delete event for pod", logging.Field{Key: "pod", Value: pod.Name}, logging.Field{Key: "namespace", Value: pod.Namespace})
			events <- kubernetes.PodEvent{
				Type: kubernetes.EventTypeDelete,
				Pod: model.Pod{
					Name:      pod.Name,
					Namespace: pod.Namespace,
					IP:        pod.Status.PodIP,
					Ready:     k.isPodReady(pod),
					NodeName:  pod.Spec.NodeName,
					Status:    string(pod.Status.Phase),
					Labels:    pod.Labels,
				},
			}
		},
	})

	k.startInformers(ctx)
	if !cache.WaitForCacheSync(ctx.Done(), informer.HasSynced) {
		return fmt.Errorf("failed to sync pod informer")
	}

	<-ctx.Done()
	k.logger.Info("WatchPods stopped")
	return ctx.Err()
}

// WatchNodes watches Kubernetes Nodes and sends events (for node IP changes).
func (k *KubernetesAdapter) WatchNodes(ctx context.Context, events chan<- kubernetes.NodeEvent) error {
	informer := k.informerFactory.Core().V1().Nodes().Informer()
	informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			node, ok := obj.(*corev1.Node)
			if !ok {
				k.logger.Error("Invalid node object in add event")
				return
			}
			k.logger.Debug("Processing add event for node", logging.Field{Key: "node", Value: node.Name})
			events <- kubernetes.NodeEvent{
				Type: kubernetes.EventTypeAdd,
				Node: model.Node{
					Name:       node.Name,
					InternalIP: getNodeInternalIP(node),
					ExternalIP: getNodeExternalIP(node),
				},
			}
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			node, ok := newObj.(*corev1.Node)
			if !ok {
				k.logger.Error("Invalid node object in update event")
				return
			}
			k.logger.Debug("Processing update event for node", logging.Field{Key: "node", Value: node.Name})
			events <- kubernetes.NodeEvent{
				Type: kubernetes.EventTypeUpdate,
				Node: model.Node{
					Name:       node.Name,
					InternalIP: getNodeInternalIP(node),
					ExternalIP: getNodeExternalIP(node),
				},
			}
		},
		DeleteFunc: func(obj interface{}) {
			node, ok := obj.(*corev1.Node)
			if !ok {
				k.logger.Error("Invalid node object in delete event")
				return
			}
			k.logger.Debug("Processing delete event for node", logging.Field{Key: "node", Value: node.Name})
			events <- kubernetes.NodeEvent{
				Type: kubernetes.EventTypeDelete,
				Node: model.Node{
					Name:       node.Name,
					InternalIP: getNodeInternalIP(node),
					ExternalIP: getNodeExternalIP(node),
				},
			}
		},
	})

	k.startInformers(ctx)
	if !cache.WaitForCacheSync(ctx.Done(), informer.HasSynced) {
		return fmt.Errorf("failed to sync node informer")
	}

	<-ctx.Done()
	k.logger.Info("WatchNodes stopped")
	return ctx.Err()
}

// GetPodsForSelector retrieves Pods matching the given label selector in a namespace.
func (k *KubernetesAdapter) GetPodsForSelector(ctx context.Context, namespace string, selector map[string]string) ([]model.Pod, error) {
	listOptions := metav1.ListOptions{}
	if len(selector) > 0 {
		listOptions.LabelSelector = labels.FormatLabels(selector)
	}

	pods, err := k.client.CoreV1().Pods(namespace).List(ctx, listOptions)
	if err != nil {
		k.logger.Error("Failed to list pods", logging.Field{Key: "error", Value: err}, logging.Field{Key: "namespace", Value: namespace})
		return nil, fmt.Errorf("failed to list pods: %w", err)
	}

	var result []model.Pod
	for _, pod := range pods.Items {
		result = append(result, model.Pod{
			Name:      pod.Name,
			Namespace: pod.Namespace,
			IP:        pod.Status.PodIP,
			Ready:     k.isPodReady(&pod),
			NodeName:  pod.Spec.NodeName,
			Status:    string(pod.Status.Phase),
			Labels:    pod.Labels,
		})
	}
	k.logger.Debug("Retrieved pods for selector", logging.Field{Key: "count", Value: len(result)}, logging.Field{Key: "namespace", Value: namespace})
	return result, nil
}

// GetNodeByName retrieves a Node by its name.
func (k *KubernetesAdapter) GetNodeByName(ctx context.Context, name string) (model.Node, error) {
	node, err := k.client.CoreV1().Nodes().Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		k.logger.Error("Failed to get node", logging.Field{Key: "error", Value: err}, logging.Field{Key: "node", Value: name})
		return model.Node{}, fmt.Errorf("failed to get node: %w", err)
	}
	k.logger.Debug("Retrieved node", logging.Field{Key: "node", Value: name})
	return model.Node{
		Name:       node.Name,
		InternalIP: getNodeInternalIP(node),
		ExternalIP: getNodeExternalIP(node),
	}, nil
}

// isMLBService checks if the service has required MLB labels and is of type LoadBalancer or NodePort.
func (k *KubernetesAdapter) isMLBService(svc *corev1.Service) bool {
	_, hasLBName := svc.Labels["mlb-loadbalancer-name"]
	_, hasUpstreamName := svc.Labels["mlb-upstream-name"]
	_, hasPort := svc.Labels["mlb-port"]
	serviceType := svc.Spec.Type
	isValidType := serviceType == corev1.ServiceTypeLoadBalancer || serviceType == corev1.ServiceTypeNodePort

	return hasLBName && hasUpstreamName && hasPort && isValidType
}

// isPodReady checks if a pod is ready based on its conditions (requirement 8).
func (k *KubernetesAdapter) isPodReady(pod *corev1.Pod) bool {
	// Check if pod has a readiness probe
	hasReadinessProbe := false
	for _, container := range pod.Spec.Containers {
		if container.ReadinessProbe != nil {
			hasReadinessProbe = true
			break
		}
	}

	if hasReadinessProbe {
		// If readiness probe exists, rely on PodReady condition
		for _, condition := range pod.Status.Conditions {
			if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue {
				return true
			}
		}
		return false
	}

	// If no readiness probe, check if all containers are ready and pod is running
	if pod.Status.Phase != corev1.PodRunning {
		return false
	}
	for _, container := range pod.Status.ContainerStatuses {
		if !container.Ready {
			return false
		}
	}
	return true
}

// getNodePort extracts the NodePort from service ports or mlb-port label.
func (k *KubernetesAdapter) getNodePort(svc *corev1.Service) int32 {
	if nodePortStr, exists := svc.Labels["mlb-port"]; exists {
		num64, err := strconv.ParseInt(nodePortStr, 10, 32)
		if err == nil {
			return int32(num64)
		}
		k.logger.Warn("Invalid mlb-port label", logging.Field{Key: "service", Value: svc.Name}, logging.Field{Key: "value", Value: nodePortStr}, logging.Field{Key: "error", Value: err})
	}
	if len(svc.Spec.Ports) > 0 && (svc.Spec.Type == corev1.ServiceTypeNodePort || svc.Spec.Type == corev1.ServiceTypeLoadBalancer) {
		return svc.Spec.Ports[0].NodePort
	}
	k.logger.Warn("No valid port found for service", logging.Field{Key: "service", Value: svc.Name})
	return 0
}

// getNodeInternalIP extracts the internal IP of a node.
func getNodeInternalIP(node *corev1.Node) string {
	for _, addr := range node.Status.Addresses {
		if addr.Type == corev1.NodeInternalIP {
			return addr.Address
		}
	}
	return ""
}

// getNodeExternalIP extracts the external IP of a node.
func getNodeExternalIP(node *corev1.Node) string {
	for _, addr := range node.Status.Addresses {
		if addr.Type == corev1.NodeExternalIP {
			return addr.Address
		}
	}
	return ""
}
