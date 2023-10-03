package utils

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func Test_GetAttrs(t *testing.T) {
	node := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "node-1",
			UID:  "node-1-uid",
		},
	}
	service := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "service-1",
			Namespace: "unit-tests",
			UID:       "service-1-uid",
		},
		Spec: v1.ServiceSpec{
			Selector: map[string]string{
				"app": "test",
			},
		},
	}
	srcPod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "src-pod",
			Namespace: "unit-tests",
			UID:       "src-pod-uid",
			Labels:    service.Spec.Selector,
		},
		Status: v1.PodStatus{
			PodIP: "1.2.3.4",
		},
		Spec: v1.PodSpec{
			NodeName: node.Name,
			Containers: []v1.Container{
				{
					Name: "src-pod-container-1",
				},
				{
					Name: "src-pod-container-2",
				},
			},
		},
	}
	destPod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "dest-pod",
			Namespace: "unit-tests",
			UID:       "dest-pod-uid",
			Labels:    service.Spec.Selector,
		},
		Status: v1.PodStatus{
			PodIP: "4.3.2.1",
		},
		Spec: v1.PodSpec{
			NodeName: node.Name,
			Containers: []v1.Container{
				{
					Name: "dest-pod-container-1",
				},
				{
					Name: "dest-pod-container-2",
				},
			},
		},
	}
	client := NewCachedK8sClient(fake.NewSimpleClientset(node, service, srcPod, destPod))
	client.Start(context.Background())

	testCases := []struct {
		name              string
		agentIP           string
		srcIP             string
		expectedSrcAttrs  map[string]interface{}
		destIP            string
		expectedDestAttrs map[string]interface{}
	}{
		{
			name:    "src & dest pods",
			agentIP: "1.1.1.1",
			srcIP:   srcPod.Status.PodIP,
			expectedSrcAttrs: map[string]interface{}{
				"source.k8s.resource.type":  "pod",
				"source.k8s.namespace.name": srcPod.Namespace,
				"source.k8s.pod.name":       srcPod.Name,
				"source.k8s.pod.uid":        srcPod.UID,
				"source.k8s.container.name": "src-pod-container-1,src-pod-container-2",
				"source.k8s.node.name":      node.Name,
				"source.k8s.node.uid":       node.UID,
				"source.k8s.service.name":   service.Name,
				"source.k8s.service.uid":    service.UID,
			},
			destIP: destPod.Status.PodIP,
			expectedDestAttrs: map[string]interface{}{
				"destination.k8s.resource.type":  "pod",
				"destination.k8s.namespace.name": destPod.Namespace,
				"destination.k8s.pod.name":       destPod.Name,
				"destination.k8s.pod.uid":        destPod.UID,
				"destination.k8s.container.name": "dest-pod-container-1,dest-pod-container-2",
				"destination.k8s.node.name":      node.Name,
				"destination.k8s.node.uid":       node.UID,
				"destination.k8s.service.name":   service.Name,
				"destination.k8s.service.uid":    service.UID,
			},
		},
		{
			name:             "src IP matches agent IP - no src pod attrs",
			agentIP:          srcPod.Status.PodIP,
			srcIP:            srcPod.Status.PodIP,
			expectedSrcAttrs: map[string]interface{}{},
			destIP:           destPod.Status.PodIP,
			expectedDestAttrs: map[string]interface{}{
				"destination.k8s.resource.type":  "pod",
				"destination.k8s.namespace.name": destPod.Namespace,
				"destination.k8s.pod.name":       destPod.Name,
				"destination.k8s.pod.uid":        destPod.UID,
				"destination.k8s.container.name": "dest-pod-container-1,dest-pod-container-2",
				"destination.k8s.node.name":      node.Name,
				"destination.k8s.node.uid":       node.UID,
				"destination.k8s.service.name":   service.Name,
				"destination.k8s.service.uid":    service.UID,
			},
		},
		{
			name:    "dest IP matches agent IP - no dest pod attrs",
			agentIP: destPod.Status.PodIP,
			srcIP:   srcPod.Status.PodIP,
			expectedSrcAttrs: map[string]interface{}{
				"source.k8s.resource.type":  "pod",
				"source.k8s.namespace.name": srcPod.Namespace,
				"source.k8s.pod.name":       srcPod.Name,
				"source.k8s.pod.uid":        srcPod.UID,
				"source.k8s.container.name": "src-pod-container-1,src-pod-container-2",
				"source.k8s.node.name":      node.Name,
				"source.k8s.node.uid":       node.UID,
				"source.k8s.service.name":   service.Name,
				"source.k8s.service.uid":    service.UID,
			},
			destIP:            destPod.Status.PodIP,
			expectedDestAttrs: map[string]interface{}{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			srcAttrs := client.GetK8sAttrsForSourceIP(tc.agentIP, tc.srcIP)
			assert.Equal(t, tc.expectedSrcAttrs, srcAttrs)

			destAttrs := client.GetK8sAttrsForDestinationIP(tc.agentIP, tc.destIP)
			assert.Equal(t, tc.expectedDestAttrs, destAttrs)
		})
	}
}
