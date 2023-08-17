package utils

import (
	"context"
	"fmt"
	"log"
	"strings"

	semconv "go.opentelemetry.io/otel/semconv/v1.20.0"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
)

func getPodByIPAddr(client *kubernetes.Clientset, ipAddr string) v1.Pod {
	pods, _ := client.CoreV1().Pods(v1.NamespaceAll).List(context.TODO(), metav1.ListOptions{})

	var matchedPod v1.Pod

	for _, pod := range pods.Items {
		if ipAddr == pod.Status.PodIP {
			matchedPod = pod
		}
	}

	return matchedPod
}

func getServiceForPod(client *kubernetes.Clientset, inputPod v1.Pod) v1.Service {
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

func getNodeByPod(client *kubernetes.Clientset, pod v1.Pod) *v1.Node {
	node, _ := client.CoreV1().Nodes().Get(context.TODO(), pod.Spec.NodeName, metav1.GetOptions{})
	return node
}

func GetK8sEventAttrs(client *kubernetes.Clientset, srcIp string, dstIp string) map[string]any {
	dstPod := getPodByIPAddr(client, dstIp)
	srcPod := getPodByIPAddr(client, srcIp)
	srcNode := getNodeByPod(client, srcPod)
	service := getServiceForPod(client, srcPod)

	k8sEventAttrs := map[string]any{
		// dest pod
		fmt.Sprintf("destination.%s", semconv.K8SPodNameKey): dstPod.Name,
		fmt.Sprintf("destination.%s", semconv.K8SPodUIDKey):  dstPod.UID,

		// source pod
		string(semconv.K8SPodNameKey): srcPod.Name,
		string(semconv.K8SPodUIDKey):  srcPod.UID,

		// namespace
		string(semconv.K8SNamespaceNameKey): srcPod.Namespace,

		// service
		// no semconv for service yet
		"k8s.service.name": service.Name,

		// node
		string(semconv.K8SNodeNameKey): srcNode.Name,
		string(semconv.K8SNodeUIDKey):  srcNode.UID,
	}
	if len(srcPod.Spec.Containers) > 0 {
		var containerNames []string
		for _, container := range srcPod.Spec.Containers {
			containerNames = append(containerNames, container.Name)
		}
		k8sEventAttrs[string(semconv.K8SContainerNameKey)] = strings.Join(containerNames, ",")
	}

	return k8sEventAttrs
}
