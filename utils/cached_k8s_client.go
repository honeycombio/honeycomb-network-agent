package utils

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
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

func NewCachedK8sClient(clientset kubernetes.Interface) *CachedK8sClient {
	factory := informers.NewSharedInformerFactory(clientset, ResyncTime)
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

// GetK8sAttrsForSourceIP returns a map of kubernetes metadata attributes for
// a given IP address. Attribute names will be prefixed with "source.".
func (c *CachedK8sClient) GetK8sAttrsForSourceIP(agentIP string, ip string) map[string]any {
	return c.getK8sAttrsForIp(agentIP, ip, "source")
}

// GetK8sAttrsForDestinationIP returns a map of kubernetes metadata attributes for
// a given IP address. Attribute names will be prefixed with "destination.".
func (c *CachedK8sClient) GetK8sAttrsForDestinationIP(agentIP string, ip string) map[string]any {
	return c.getK8sAttrsForIp(agentIP, ip, "destination")
}

// getK8sAttrsForIp returns a map of kubernetes metadata attributes for a given IP address.
//
// Provide a prefix to prepend to the attribute names, example: "source" or "destination".
//
// If the IP address is not found in the kubernetes cache, an empty map is returned.
func (client *CachedK8sClient) getK8sAttrsForIp(agentIP string, ip string, prefix string) map[string]any {
	k8sAttrs := map[string]any{}

	if ip == "" {
		return k8sAttrs
	}

	// Try add k8s attributes for source and destination when they are not the agent pod IP.
	// Because we use hostnetwork in deployments, the agent pod IP and node IP are the same and we
	// can't distinguish between the two, or any other pods that is also running with hostnetwork.
	if ip == agentIP {
		return k8sAttrs
	}

	if prefix != "" {
		prefix = fmt.Sprintf("%s.", prefix)
	}

	if pod := client.GetPodByIPAddr(ip); pod != nil {
		k8sAttrs[prefix+string(semconv.K8SPodNameKey)] = pod.Name
		k8sAttrs[prefix+string(semconv.K8SPodUIDKey)] = pod.UID
		k8sAttrs[prefix+string(semconv.K8SNamespaceNameKey)] = pod.Namespace

		if len(pod.Spec.Containers) > 0 {
			var containerNames []string
			for _, container := range pod.Spec.Containers {
				containerNames = append(containerNames, container.Name)
			}
			k8sAttrs[prefix+string(semconv.K8SContainerNameKey)] = strings.Join(containerNames, ",")
		}

		if node := client.GetNodeForPod(pod); node != nil {
			k8sAttrs[prefix+string(semconv.K8SNodeNameKey)] = node.Name
			k8sAttrs[prefix+string(semconv.K8SNodeUIDKey)] = node.UID
		}

		if service := client.GetServiceForPod(pod); service != nil {
			// no semconv for service yet
			k8sAttrs[prefix+"k8s.service.name"] = service.Name
			k8sAttrs[prefix+"k8s.service.uid"] = service.UID
		}
	}

	return k8sAttrs
}
