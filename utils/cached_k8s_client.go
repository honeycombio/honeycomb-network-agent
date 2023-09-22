package utils

import (
	"context"
	"time"

	"github.com/rs/zerolog/log"
	v1 "k8s.io/api/core/v1"
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

	podInformer.AddIndexers(map[string]cache.IndexFunc{
		podByIPIndexName: func(obj interface{}) ([]string, error) {
			pod := obj.(*v1.Pod)
			return []string{pod.Status.PodIP}, nil
		},
	})
	serviceInformer.AddIndexers(map[string]cache.IndexFunc{
		serviceByPodNameIndex: func(obj interface{}) ([]string, error) {
			service := obj.(*v1.Service)
			return []string{service.Spec.Selector["pod"]}, nil
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
func (c *CachedK8sClient) GetServiceForPod(pod *v1.Pod) *v1.Service {
	log.Info().Str("pod", pod.Name).Msg("Getting service for pod")
	val, err := c.serviceInformer.GetIndexer().ByIndex(serviceByPodNameIndex, pod.Name)
	if err != nil {
		log.Err(err).Msg("failed to get service for pod")
		return nil
	}
	if len(val) == 0 {
		log.Info().Str("pod", pod.Name).Msg("No service found for pod")
		return nil
	}
	service := val[0].(*v1.Service)
	log.Info().Str("pod", pod.Name).Str("service", service.Name).Msg("Found service for pod")
	return service
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
