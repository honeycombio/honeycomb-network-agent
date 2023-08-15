package utils

import (
	"context"
	"log"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
)

func GetPodByIPAddr(client *kubernetes.Clientset, ipAddr string) v1.Pod {
	pods, _ := client.CoreV1().Pods(v1.NamespaceAll).List(context.TODO(), metav1.ListOptions{})

	var matchedPod v1.Pod

	for _, pod := range pods.Items {
		if ipAddr == pod.Status.PodIP {
			matchedPod = pod
		}
	}

	return matchedPod
}

func GetServiceForPod(client *kubernetes.Clientset, inputPod v1.Pod) v1.Service {
	// get list of services
	services, _ := client.CoreV1().Services(v1.NamespaceAll).List(context.TODO(), metav1.ListOptions{})
	var matchedService v1.Service
	// loop over services
	for _, service := range services.Items {
		set := labels.Set(service.Spec.Selector)
		listOptions := metav1.ListOptions{LabelSelector: set.AsSelector().String()}
		pods, err := client.CoreV1().Pods(v1.NamespaceAll).List(context.TODO(), listOptions)
		if err != nil {
			log.Println(err)
		}
		for _, pod := range pods.Items {
			if pod.Name == inputPod.Name {
				matchedService = service
			}
		}
	}

	return matchedService
}

func GetNodeByPod(client *kubernetes.Clientset, pod v1.Pod) *v1.Node {
	node, _ := client.CoreV1().Nodes().Get(context.TODO(), pod.Spec.NodeName, metav1.GetOptions{})
	return node
}
