package utils

import (
	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
)

func FakeNewCachedK8sClient(client *fake.Clientset) *CachedK8sClient {
	factory := informers.NewSharedInformerFactory(client, ResyncTime)
	podInformer := factory.Core().V1().Pods().Informer()
	serviceInformer := factory.Core().V1().Services().Informer()
	nodeInformer := factory.Core().V1().Nodes().Informer()

	return &CachedK8sClient{
		factory:         factory,
		nodeInformer:    nodeInformer,
		podInformer:     podInformer,
		serviceInformer: serviceInformer,
	}
}

func (c *CachedK8sClient) FakeGetPodByIPAddr(ipAddr string) *v1.Pod {
	pods := c.GetPods()
	var matchedPod *v1.Pod
	for _, pod := range pods {
		if ipAddr == pod.Status.PodIP || ipAddr == pod.Status.HostIP {
			matchedPod = pod
		}
	}
	return matchedPod
}
