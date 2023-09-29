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
	ResyncTime      = time.Minute * 5
	byIPIndex       = "podIP"
	nodeByNameIndex = "nodeName"
)

type CachedK8sClient struct {
	factory         informers.SharedInformerFactory
	nodeInformer    cache.SharedIndexInformer
	podInformer     cache.SharedIndexInformer
	serviceInformer cache.SharedIndexInformer
}

func NewCachedK8sClient(clientset kubernetes.Interface) *CachedK8sClient {
	factory := informers.NewSharedInformerFactory(clientset, ResyncTime)
	podInformer := factory.Core().V1().Pods().Informer()
	serviceInformer := factory.Core().V1().Services().Informer()
	nodeInformer := factory.Core().V1().Nodes().Informer()

	podInformer.AddIndexers(map[string]cache.IndexFunc{
		byIPIndex: func(obj interface{}) ([]string, error) {
			pod := obj.(*v1.Pod)
			return []string{pod.Status.PodIP}, nil
		},
	})
	serviceInformer.AddIndexers(map[string]cache.IndexFunc{
		byIPIndex: func(obj interface{}) ([]string, error) {
			service := obj.(*v1.Service)
			return []string{service.Spec.ClusterIP}, nil
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
	val, err := c.podInformer.GetIndexer().ByIndex(byIPIndex, ipAddr)
	if err != nil {
		log.Err(err).Msg("Error getting pod by IP")
		return nil
	}
	if len(val) == 0 {
		return nil
	}
	return val[0].(*v1.Pod)
}

// GetServiceByIPAddr returns the service with the given IP address
func (c *CachedK8sClient) GetServiceByIPAddr(ipAddr string) *v1.Service {
	val, err := c.serviceInformer.GetIndexer().ByIndex(byIPIndex, ipAddr)
	if err != nil {
		log.Err(err).Msg("Error getting service by IP")
		return nil
	}
	if len(val) == 0 {
		return nil
	}
	return val[0].(*v1.Service)
}

// GetServiceForPod returns the service that the given pod is associated with
func (c *CachedK8sClient) GetServiceForPod(pod *v1.Pod) *v1.Service {
	podLabels := labels.Set(pod.Labels)
	for _, item := range c.serviceInformer.GetStore().List() {
		service := item.(*v1.Service)
		// Ignore services without selectors
		if service.Spec.Selector == nil {
			continue
		}
		serviceSelector := labels.SelectorFromSet(service.Spec.Selector)
		if serviceSelector.Matches(podLabels) {
			return service
		}
	}
	return nil
}

// GetNodeByName returns the node with the given name
func (c *CachedK8sClient) GetNodeForPod(pod *v1.Pod) *v1.Node {
	val, err := c.nodeInformer.GetIndexer().ByIndex(nodeByNameIndex, pod.Spec.NodeName)
	if err != nil {
		log.Err(err).Msg("Error getting node by name")
		return nil
	}
	if len(val) == 0 {
		return nil
	}
	return val[0].(*v1.Node)
}
