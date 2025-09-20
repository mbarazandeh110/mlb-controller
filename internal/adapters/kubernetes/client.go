// internal/adapters/kubernetes/client.go
package kubernetes

import (
	"context"
	"fmt"
	"os"

	"mlb-controller/internal/domain/config"
	"mlb-controller/internal/domain/model"
	"mlb-controller/internal/ports/logging"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
)

// KubernetesClient implements the KubernetesAdapter interface.
type KubernetesClient struct {
	clientset       *kubernetes.Clientset
	informerFactory informers.SharedInformerFactory
	log             logging.Logger
}

// NewKubernetesClient creates a new KubernetesClient instance.
func NewKubernetesClient(log logging.Logger, kubeConfig config.KubernetesConfig) (*KubernetesClient, error) {
	// Access the fields from the kubeConf struct
	config, err := buildConfig(kubeConfig.KubernetesConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to build kubernetes config: %w", err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes clientset: %w", err)
	}

	// Use the ResyncPeriod from the config struct
	informerFactory := informers.NewSharedInformerFactory(clientset, kubeConfig.ResyncPeriod)

	return &KubernetesClient{
		clientset:       clientset,
		informerFactory: informerFactory,
		log:             log,
	}, nil
}

// buildConfig creates a Kubernetes config object.
func buildConfig(kubeconfigPath string) (*rest.Config, error) {
	// If a kubeconfig path is provided (from the config file or env var),
	if kubeconfigPath == "" {
		kubeconfigPath = os.Getenv("KUBECONFIG")
	}

	// use it to build the configuration.
	if kubeconfigPath != "" {
		return clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	}

	// If no kubeconfig path is provided, assume we are running inside the cluster
	// and use the in-cluster configuration.
	return rest.InClusterConfig()
}

// StartInformer starts the informers and returns immediately.
func (c *KubernetesClient) StartInformer(ctx context.Context) error {
	c.informerFactory.Start(ctx.Done())
	return nil
}

// WaitForCacheSync waits for the caches to be synced.
func (c *KubernetesClient) WaitForCacheSync(ctx context.Context) bool {
	// The WaitForCacheSync function returns a map, not a bool.
	// The correct way to wait for the sync is to use cache.WaitForCacheSync.
	return cache.WaitForCacheSync(ctx.Done(),
		c.informerFactory.Core().V1().Services().Informer().HasSynced,
		c.informerFactory.Core().V1().Pods().Informer().HasSynced,
		c.informerFactory.Core().V1().Nodes().Informer().HasSynced)
}

// GetServices returns a list of all services from the informer cache.
func (c *KubernetesClient) GetServices() ([]model.Service, error) {
	// The List method returns two values: a slice and an error.
	serviceList, err := c.informerFactory.Core().V1().Services().Lister().List(labels.Everything())
	if err != nil {
		return nil, fmt.Errorf("failed to list services from cache: %w", err)
	}

	var services []model.Service
	for _, s := range serviceList {
		services = append(services, model.Service{
			Name:        s.Name,
			Namespace:   s.Namespace,
			Labels:      s.Labels,
			Selector:    s.Spec.Selector,
			ServiceType: string(s.Spec.Type),
			NodePort:    getNodePort(s),
		})
	}
	return services, nil
}

func getNodePort(svc *corev1.Service) int32 {
	if svc.Spec.Type == corev1.ServiceTypeNodePort {
		for _, port := range svc.Spec.Ports {
			if port.NodePort > 0 {
				return port.NodePort
			}
		}
	}
	return 0
}

// GetPodsBySelector returns pods from the informer cache that match the selector.
func (c *KubernetesClient) GetPodsBySelector(namespace string, selector map[string]string) ([]model.Pod, error) {
	labelSelector := labels.SelectorFromSet(selector)
	pods, err := c.informerFactory.Core().V1().Pods().Lister().Pods(namespace).List(labelSelector)
	if err != nil {
		return nil, fmt.Errorf("failed to get pods for selector %v in namespace %s: %w", selector, namespace, err)
	}

	var result []model.Pod
	for _, p := range pods {
		result = append(result, model.Pod{
			Name:      p.Name,
			Namespace: p.Namespace,
			NodeName:  p.Spec.NodeName,
			IP:        p.Status.PodIP,
			Status:    string(p.Status.Phase),
			Ready:     isPodReady(p),
		})
	}
	return result, nil
}

func isPodReady(pod *corev1.Pod) bool {
	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

// GetNode returns a node by its name from the informer cache.
func (c *KubernetesClient) GetNode(name string) (model.Node, error) {
	node, err := c.informerFactory.Core().V1().Nodes().Lister().Get(name)
	if err != nil {
		return model.Node{}, fmt.Errorf("failed to get node %s: %w", name, err)
	}

	return model.Node{
		Name:       node.Name,
		InternalIP: getAddress(node.Status.Addresses, corev1.NodeInternalIP),
		ExternalIP: getAddress(node.Status.Addresses, corev1.NodeExternalIP),
		ClusterIP:  getAddress(node.Status.Addresses, corev1.NodeInternalIP), // Assuming cluster IP is the same as internal IP for simplicity.
	}, nil
}

func getAddress(addresses []corev1.NodeAddress,
	addressType corev1.NodeAddressType) string {
	for _, addr := range addresses {
		if addr.Type == addressType {
			return addr.Address
		}
	}
	return ""
}
