package utils

import (
	"fmt"
	"strings"

	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
)

const (
	k8sResourceType        = "k8s.resource.type"
	k8sResourceTypePod     = "pod"
	k8sResourceTypeService = "service"
	k8sServiceName         = "k8s.service.name"
	k8sServiceUID          = "k8s.service.uid"
)

// GetK8sAttrsForSourceIp returns a map of kubernetes metadata attributes for
// a given IP address. Attribute names will be prefixed with "source.".
func GetK8sAttrsForSourceIp(client *CachedK8sClient, ip string) map[string]any {
	return GetK8sAttrsForIp(client, ip, "source")
}

// GetK8sAttrsForDestinationIp returns a map of kubernetes metadata attributes for
// a given IP address. Attribute names will be prefixed with "destination.".
func GetK8sAttrsForDestinationIp(client *CachedK8sClient, ip string) map[string]any {
	return GetK8sAttrsForIp(client, ip, "destination")
}

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
		k8sAttrs[prefix+k8sResourceType] = k8sResourceTypePod
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
			k8sAttrs[prefix+k8sServiceName] = service.Name
			k8sAttrs[prefix+k8sServiceUID] = service.UID
		}
	} else if service := client.GetServiceByIPAddr(ip); service != nil {
		k8sAttrs[prefix+k8sResourceType] = k8sResourceTypeService
		k8sAttrs[prefix+string(semconv.K8SNamespaceNameKey)] = service.Namespace
		// no semconv for service yet
		k8sAttrs[prefix+k8sServiceName] = service.Name
		k8sAttrs[prefix+k8sServiceUID] = service.UID
	}

	return k8sAttrs
}
