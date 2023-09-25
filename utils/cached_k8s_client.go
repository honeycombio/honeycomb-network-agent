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
	podByIPIndex    = "podIP"
	nodeByNameIndex = "nodeName"
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
		podByIPIndex: func(obj interface{}) ([]string, error) {
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

// GetPodByIPAddr returns the pod with the given IP address
func (c *CachedK8sClient) GetPodByIPAddr(ipAddr string) *v1.Pod {
	val, err := c.podInformer.GetIndexer().ByIndex(podByIPIndex, ipAddr)
	if err != nil {
		log.Err(err).Msg("Error getting pod by IP")
		return nil
	}
	if len(val) == 0 {
		return nil
	}
	return val[0].(*v1.Pod)
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
func (c *CachedK8sClient) GetNodeByName(nodeName string) *v1.Node {
	val, err := c.nodeInformer.GetIndexer().ByIndex(nodeByNameIndex, nodeName)
	if err != nil {
		log.Err(err).Msg("Error getting node by name")
		return nil
	}
	if len(val) == 0 {
		return nil
	}
	return val[0].(*v1.Node)
}
