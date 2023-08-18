package utils

import (
	"context"
	"time"

	"github.com/rs/zerolog/log"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/informers"
	informerV1 "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
)

type CachedK8sClient struct {
	factory         informers.SharedInformerFactory
	nodeInformer    informerV1.NodeInformer
	podInformer     informerV1.PodInformer
	serviceInformer informerV1.ServiceInformer
}

func NewCachedK8sClient(ctx context.Context, client *kubernetes.Clientset) *CachedK8sClient {
	factory := informers.NewSharedInformerFactory(client, time.Minute*1)
	podInformer := factory.Core().V1().Pods()
	serviceInformer := factory.Core().V1().Services()
	nodeInformer := factory.Core().V1().Nodes()
	factory.Start(ctx.Done())

	sync := factory.WaitForCacheSync(ctx.Done())
	for v, ok := range sync {
		if !ok {
			log.Error().Str("msg", v.Name()).Msg("failed to sync informer")
		}
	}

	return &CachedK8sClient{
		factory:         factory,
		nodeInformer:    nodeInformer,
		podInformer:     podInformer,
		serviceInformer: serviceInformer,
	}
}

func (c *CachedK8sClient) Start(ctx context.Context) {
	c.factory.Start(ctx.Done())
}

func (c *CachedK8sClient) GetNodes() ([]*v1.Node, error) {
	return c.nodeInformer.Lister().List(labels.Everything())
}

func (c *CachedK8sClient) GetPods() ([]*v1.Pod, error) {
	return c.podInformer.Lister().Pods("").List(labels.Everything())
}

func (c *CachedK8sClient) GetPodsWithSelector(selector labels.Selector) ([]*v1.Pod, error) {
	return c.podInformer.Lister().List(selector)
}

func (c *CachedK8sClient) GetServices() ([]*v1.Service, error) {
	return c.serviceInformer.Lister().List(labels.Everything())
}

func (c *CachedK8sClient) GetServicesWithSelector(selector labels.Selector) ([]*v1.Service, error) {
	return c.serviceInformer.Lister().List(selector)
}

func (c *CachedK8sClient) GetPodByIPAddr(ipAddr string) *v1.Pod {
	pods, err := c.GetPods()
	log.Info().Any("pods", pods).Msg("Got these pods")
	if err != nil {
		log.Error().Str("msg", "failed to get pod by ip address").Msg("failed to get pods")
	}
	var matchedPod *v1.Pod
	for _, pod := range pods {
		if ipAddr == pod.Status.PodIP {
			matchedPod = pod
		}
	}
	return matchedPod
}

func (monitor *CachedK8sClient) GetServiceForPod(inputPod *v1.Pod) *v1.Service {
	services, err := monitor.GetServices()
	if err != nil {
		log.Error().Str("msg", "failed to get service for pod").Msg("failed to get services")
	}
	var matchedService *v1.Service
	for _, service := range services {
		set := labels.Set(service.Spec.Selector)
		pods, err := monitor.GetPodsWithSelector(set.AsSelector())
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

func (m *CachedK8sClient) GetNodeByPod(pod *v1.Pod) *v1.Node {
	nodes, err := m.GetNodes()
	if err != nil {
		log.Error().Str("msg", "failed to get node by pod").Msg("failed to get nodes")
	}
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
