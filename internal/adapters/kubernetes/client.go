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

type KubernetesAdapter struct {
	client          k8sclient.Interface
	logger          logging.Logger
	config          config.KubernetesConfig
	informerFactory informers.SharedInformerFactory
	started         bool
}

func NewKubernetesAdapter(k8sCfg config.KubernetesConfig, logger logging.Logger) (*KubernetesAdapter, error) {
	kubeConfig, err := clientcmd.BuildConfigFromFlags("", k8sCfg.KubernetesConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to build kubernetes config: %w", err)
	}
	client, err := k8sclient.NewForConfig(kubeConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	informerFactory := informers.NewSharedInformerFactory(client, k8sCfg.ResyncPeriod)

	return &KubernetesAdapter{
		client:          client,
		logger:          logger,
		config:          k8sCfg,
		informerFactory: informerFactory,
		started:         false,
	}, nil
}

func (k *KubernetesAdapter) startInformers(ctx context.Context) {
	if !k.started {
		go k.informerFactory.Start(ctx.Done())
		k.started = true
		k.logger.Info("Kubernetes informers started")
	}
}

// استفاده از Non-blocking Send برای کانال‌ها
func sendServiceEvent(events chan<- kubernetes.ServiceEvent, event kubernetes.ServiceEvent) {
	select {
	case events <- event:
	default:
	}
}

func (k *KubernetesAdapter) WatchServices(ctx context.Context, requiredLabels map[string]string, events chan<- kubernetes.ServiceEvent) error {
	informer := k.informerFactory.Core().V1().Services().Informer()
	informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			svc, ok := obj.(*corev1.Service)
			if ok && k.isMLBService(svc) {
				sendServiceEvent(events, kubernetes.ServiceEvent{
					Type:    kubernetes.EventTypeAdd,
					Service: k.mapService(svc),
				})
			}
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			oldSvc, okOld := oldObj.(*corev1.Service)
			newSvc, okNew := newObj.(*corev1.Service)
			if !okNew {
				return
			}
			wasMLB := okOld && k.isMLBService(oldSvc)
			isMLB := k.isMLBService(newSvc)

			if wasMLB && !isMLB {
				sendServiceEvent(events, kubernetes.ServiceEvent{
					Type:    kubernetes.EventTypeDelete,
					Service: k.mapService(oldSvc),
				})
			} else if isMLB {
				sendServiceEvent(events, kubernetes.ServiceEvent{
					Type:    kubernetes.EventTypeUpdate,
					Service: k.mapService(newSvc),
				})
			}
		},
		DeleteFunc: func(obj interface{}) {
			if tombstone, ok := obj.(cache.DeletedFinalStateUnknown); ok {
				obj = tombstone.Obj
			}
			svc, ok := obj.(*corev1.Service)
			if ok && k.isMLBService(svc) {
				sendServiceEvent(events, kubernetes.ServiceEvent{
					Type:    kubernetes.EventTypeDelete,
					Service: k.mapService(svc),
				})
			}
		},
	})

	k.startInformers(ctx)
	if !cache.WaitForCacheSync(ctx.Done(), informer.HasSynced) {
		return fmt.Errorf("failed to sync service informer")
	}

	<-ctx.Done()
	return ctx.Err()
}

func sendPodEvent(events chan<- kubernetes.PodEvent, event kubernetes.PodEvent) {
	select {
	case events <- event:
	default:
	}
}

func (k *KubernetesAdapter) WatchPods(ctx context.Context, events chan<- kubernetes.PodEvent) error {
	informer := k.informerFactory.Core().V1().Pods().Informer()
	informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			if pod, ok := obj.(*corev1.Pod); ok {
				sendPodEvent(events, kubernetes.PodEvent{
					Type: kubernetes.EventTypeAdd,
					Pod:  k.mapPod(pod),
				})
			}
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			if pod, ok := newObj.(*corev1.Pod); ok {
				sendPodEvent(events, kubernetes.PodEvent{
					Type: kubernetes.EventTypeUpdate,
					Pod:  k.mapPod(pod),
				})
			}
		},
		DeleteFunc: func(obj interface{}) {
			if tombstone, ok := obj.(cache.DeletedFinalStateUnknown); ok {
				obj = tombstone.Obj
			}
			if pod, ok := obj.(*corev1.Pod); ok {
				sendPodEvent(events, kubernetes.PodEvent{
					Type: kubernetes.EventTypeDelete,
					Pod:  k.mapPod(pod),
				})
			}
		},
	})

	k.startInformers(ctx)
	if !cache.WaitForCacheSync(ctx.Done(), informer.HasSynced) {
		return fmt.Errorf("failed to sync pod informer")
	}

	<-ctx.Done()
	return ctx.Err()
}

func sendNodeEvent(events chan<- kubernetes.NodeEvent, event kubernetes.NodeEvent) {
	select {
	case events <- event:
	default:
	}
}

func (k *KubernetesAdapter) WatchNodes(ctx context.Context, events chan<- kubernetes.NodeEvent) error {
	informer := k.informerFactory.Core().V1().Nodes().Informer()
	informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			if node, ok := obj.(*corev1.Node); ok {
				sendNodeEvent(events, kubernetes.NodeEvent{
					Type: kubernetes.EventTypeAdd,
					Node: k.mapNode(node),
				})
			}
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			if node, ok := newObj.(*corev1.Node); ok {
				sendNodeEvent(events, kubernetes.NodeEvent{
					Type: kubernetes.EventTypeUpdate,
					Node: k.mapNode(node),
				})
			}
		},
		DeleteFunc: func(obj interface{}) {
			if tombstone, ok := obj.(cache.DeletedFinalStateUnknown); ok {
				obj = tombstone.Obj
			}
			if node, ok := obj.(*corev1.Node); ok {
				sendNodeEvent(events, kubernetes.NodeEvent{
					Type: kubernetes.EventTypeDelete,
					Node: k.mapNode(node),
				})
			}
		},
	})

	k.startInformers(ctx)
	if !cache.WaitForCacheSync(ctx.Done(), informer.HasSynced) {
		return fmt.Errorf("failed to sync node informer")
	}

	<-ctx.Done()
	return ctx.Err()
}

// جایگزین کردن فراخوانی مستقیم API Server با خواندن از حافظه محلی کش (Lister)
func (k *KubernetesAdapter) GetPodsForSelector(ctx context.Context, namespace string, selector map[string]string) ([]model.Pod, error) {
	sel := labels.SelectorFromSet(selector)
	pods, err := k.informerFactory.Core().V1().Pods().Lister().Pods(namespace).List(sel)
	if err != nil {
		return nil, fmt.Errorf("failed to list pods from cache: %w", err)
	}

	var result []model.Pod
	for _, pod := range pods {
		result = append(result, k.mapPod(pod))
	}
	return result, nil
}

func (k *KubernetesAdapter) GetNodeByName(ctx context.Context, name string) (model.Node, error) {
	node, err := k.informerFactory.Core().V1().Nodes().Lister().Get(name)
	if err != nil {
		// Fallback to API if not in cache
		node, err = k.client.CoreV1().Nodes().Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return model.Node{}, fmt.Errorf("failed to get node: %w", err)
		}
	}
	return k.mapNode(node), nil
}

func (k *KubernetesAdapter) HasSynced() bool {
	if k.informerFactory == nil {
		return false
	}
	return k.informerFactory.Core().V1().Services().Informer().HasSynced() &&
		k.informerFactory.Core().V1().Pods().Informer().HasSynced() &&
		k.informerFactory.Core().V1().Nodes().Informer().HasSynced()
}

func (k *KubernetesAdapter) mapService(svc *corev1.Service) model.Service {
	return model.Service{
		Name:        svc.Name,
		Namespace:   svc.Namespace,
		Labels:      svc.Labels,
		Selector:    svc.Spec.Selector,
		ServiceType: string(svc.Spec.Type),
		NodePort:    k.getNodePort(svc),
	}
}

func (k *KubernetesAdapter) mapPod(pod *corev1.Pod) model.Pod {
	return model.Pod{
		Name:      pod.Name,
		Namespace: pod.Namespace,
		IP:        pod.Status.PodIP,
		Ready:     k.isPodReady(pod),
		NodeName:  pod.Spec.NodeName,
		Status:    string(pod.Status.Phase),
		Labels:    pod.Labels,
	}
}

func (k *KubernetesAdapter) mapNode(node *corev1.Node) model.Node {
	return model.Node{
		Name:       node.Name,
		InternalIP: getNodeInternalIP(node),
		ExternalIP: getNodeExternalIP(node),
	}
}

func (k *KubernetesAdapter) isMLBService(svc *corev1.Service) bool {
	_, hasLBName := svc.Labels["mlb-loadbalancer-name"]
	_, hasUpstreamName := svc.Labels["mlb-upstream-name"]
	_, hasPort := svc.Labels["mlb-port"]
	serviceType := svc.Spec.Type
	return hasLBName && hasUpstreamName && hasPort && (serviceType == corev1.ServiceTypeLoadBalancer || serviceType == corev1.ServiceTypeNodePort)
}

func (k *KubernetesAdapter) isPodReady(pod *corev1.Pod) bool {
	hasReadinessProbe := false
	for _, container := range pod.Spec.Containers {
		if container.ReadinessProbe != nil {
			hasReadinessProbe = true
			break
		}
	}

	if hasReadinessProbe {
		for _, condition := range pod.Status.Conditions {
			if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue {
				return true
			}
		}
		return false
	}

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

func (k *KubernetesAdapter) getNodePort(svc *corev1.Service) int32 {
	if nodePortStr, exists := svc.Labels["mlb-port"]; exists {
		num64, err := strconv.ParseInt(nodePortStr, 10, 32)
		if err == nil {
			return int32(num64)
		}
	}
	if len(svc.Spec.Ports) > 0 && (svc.Spec.Type == corev1.ServiceTypeNodePort || svc.Spec.Type == corev1.ServiceTypeLoadBalancer) {
		return svc.Spec.Ports[0].NodePort
	}
	return 0
}

func getNodeInternalIP(node *corev1.Node) string {
	for _, addr := range node.Status.Addresses {
		if addr.Type == corev1.NodeInternalIP {
			return addr.Address
		}
	}
	return ""
}

func getNodeExternalIP(node *corev1.Node) string {
	for _, addr := range node.Status.Addresses {
		if addr.Type == corev1.NodeExternalIP {
			return addr.Address
		}
	}
	return ""
}
