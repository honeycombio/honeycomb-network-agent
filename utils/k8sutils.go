package utils

import (
	"fmt"
	"strings"

	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
)

// GetK8sAttrsForIp returns a map of kubernetes metadata attributes for a given IP address.
//
// Provide a prefix to prepend to the attribute names, example: "source" or "destination".
//
// If the IP address is not found in the kubernetes cache, an empty map is returned.
func GetK8sAttrsForIp(client *CachedK8sClient, ip string, prefix string) map[string]any {
	k8sAttrs := map[string]any{}

	if ip == "" {
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

		if node := client.GetNodeByName(pod.Spec.NodeName); node != nil {
			k8sAttrs[prefix+string(semconv.K8SNodeNameKey)] = node.Name
			k8sAttrs[prefix+string(semconv.K8SNodeUIDKey)] = node.UID
		}

		if service := client.GetServiceForPod(pod); service != nil {
			// no semconv for service yet
			k8sAttrs["k8s.service.name"] = service.Name
		}
	}

	return k8sAttrs
}
