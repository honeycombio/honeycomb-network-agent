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
	ResyncTime            = time.Minute * 5
	podByIPIndexName      = "podIP"
	serviceByPodNameIndex = "podName"
	nodeByNameIndex       = "nodeName"
)

type CachedK8sClient struct {
	factory         informers.SharedInformerFactory
	nodeInformer    cache.SharedIndexInformer
	podInformer     cache.SharedIndexInformer
	serviceInformer cache.SharedIndexInformer
}

func NewCachedK8sClient(client *kubernetes.Clientset) *CachedK8sClient {
	factory := informers.NewSharedInformerFactory(client, ResyncTime)
	podInformer := factory.Core().V1().Pods().Informer()
	serviceInformer := factory.Core().V1().Services().Informer()
	nodeInformer := factory.Core().V1().Nodes().Informer()
	// TODO: add index for service by pod name

	podInformer.AddIndexers(map[string]cache.IndexFunc{
		podByIPIndexName: func(obj interface{}) ([]string, error) {
			pod := obj.(*v1.Pod)
			return []string{pod.Status.PodIP}, nil
		},
	})
	nodeInformer.AddIndexers(map[string]cache.IndexFunc{
		nodeByNameIndex: func(obj interface{}) ([]string, error) {
			node := obj.(*v1.Node)
			return []string{node.Name}, nil
		},
	})

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

func (c *CachedK8sClient) GetPods() []*v1.Pod {
	var pods []*v1.Pod
	items := c.podInformer.GetStore().List()
	for _, pod := range items {
		pods = append(pods, pod.(*v1.Pod))
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

// GetPodByIPAddr returns the pod with the given IP address
func (c *CachedK8sClient) GetPodByIPAddr(ipAddr string) *v1.Pod {
	val, err := c.podInformer.GetIndexer().ByIndex(podByIPIndexName, ipAddr)
	if err != nil {
		log.Err(err).Msg("failed to get pod by IP address")
		return nil
	}
	if len(val) == 0 {
		return nil
	}
	return val[0].(*v1.Pod)
}

// GetServiceForPod returns the service that the given pod is associated with
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

// GetNodeByName returns the node with the given name
func (c *CachedK8sClient) GetNodeByName(nodeName string) *v1.Node {
	val, err := c.nodeInformer.GetIndexer().ByIndex(nodeByNameIndex, nodeName)
	if err != nil {
		return nil
	}
	if len(val) == 0 {
		log.Info().Str("node", nodeName).Msg("No node found by name")
		return nil
	}
	return val[0].(*v1.Node)
}
