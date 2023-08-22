package utils

import (
	"context"
	"time"

	"github.com/rs/zerolog/log"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

const (
	ResyncTime = time.Minute * 5
)

type CachedK8sClient struct {
	factory         informers.SharedInformerFactory
	nodeInformer    cache.SharedInformer
	podInformer     cache.SharedInformer
	serviceInformer cache.SharedInformer
}

func NewCachedK8sClient(client *kubernetes.Clientset) *CachedK8sClient {
	factory := informers.NewSharedInformerFactory(client, ResyncTime)
	podInformer := factory.Core().V1().Pods().Informer()
	serviceInformer := factory.Core().V1().Services().Informer()
	nodeInformer := factory.Core().V1().Nodes().Informer()
	// TODO: add custom indexes to improve lookup speed
	// - podinformer: pod IP
	// - serviceinformer: by pod name
	// - nodeinformer: by pod IP

	return &CachedK8sClient{
		factory:         factory,
		nodeInformer:    nodeInformer,
		podInformer:     podInformer,
		serviceInformer: serviceInformer,
	}
}

func (c *CachedK8sClient) Start(ctx context.Context) {
	c.factory.Start(ctx.Done())
	c.factory.WaitForCacheSync(ctx.Done())
}

func (c *CachedK8sClient) GetNodes() []*v1.Node {
	var nodes []*v1.Node
	items := c.nodeInformer.GetStore().List()
	for _, node := range items {
		nodes = append(nodes, node.(*v1.Node))
	}
	return nodes
}

func (c *CachedK8sClient) GetPods() []*v1.Pod {
	var pods []*v1.Pod
	items := c.podInformer.GetStore().List()
	for _, pod := range items {
		pods = append(pods, pod.(*v1.Pod))
	}
	return pods
}

func (c *CachedK8sClient) GetPodsByNamespace(namespace string) []*v1.Pod {
	var pods []*v1.Pod
	for _, pod := range c.GetPods() {
		if pod.Namespace == namespace {
			pods = append(pods, pod)
		}
	}
	return pods
}

func (c *CachedK8sClient) GetServices() ([]*v1.Service, error) {
	var services []*v1.Service
	items := c.serviceInformer.GetStore().List()
	for _, service := range items {
		services = append(services, service.(*v1.Service))
	}
	return services, nil
}

func (c *CachedK8sClient) GetPodsWithSelector(selector labels.Selector) ([]*v1.Pod, error) {
	pods := c.GetPods()
	var matchedPods []*v1.Pod
	for _, pod := range pods {
		if selector.Matches(labels.Set(pod.Labels)) {
			matchedPods = append(matchedPods, pod)
		}
	}
	return matchedPods, nil
}

func (c *CachedK8sClient) GetPodByIPAddr(ipAddr string) *v1.Pod {
	pods := c.GetPods()
	var matchedPod *v1.Pod
	for _, pod := range pods {
		if ipAddr == pod.Status.PodIP || ipAddr == pod.Status.HostIP {
			matchedPod = pod
		}
	}
	return matchedPod
}

func (c *CachedK8sClient) GetServiceForPod(inputPod *v1.Pod) *v1.Service {
	services, err := c.GetServices()
	if err != nil {
		log.Error().Err(err).Msg("Failed to get service for pod")
	}
	var matchedService *v1.Service
	for _, service := range services {
		set := labels.Set(service.Spec.Selector)
		pods, err := c.GetPodsWithSelector(set.AsSelector())
		if err != nil {
			log.Error().Str("msg", "failed to get service for pod").Msg("failed to get pods")
		}
		for _, pod := range pods {
			if pod.Name == inputPod.Name {
				matchedService = service
			}
		}
	}
	return matchedService
}

func (c *CachedK8sClient) GetNodeByPod(pod *v1.Pod) *v1.Node {
	nodes := c.GetNodes()
	var matchedNode *v1.Node
	for _, node := range nodes {
		for _, addr := range node.Status.Addresses {
			if addr.Address == pod.Status.HostIP {
				matchedNode = node
			}
		}
	}
	return matchedNode
}
