package utils

import (
	"fmt"
	"strings"

	"github.com/rs/zerolog/log"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
)

func GetK8sEventAttrs(client *CachedK8sClient, srcIp string, dstIp string) map[string]any {
	log.Debug().
		Str("src_ip", srcIp).
		Str("dst_ip", dstIp).
		Msg("Getting k8s event attrs")

	k8sEventAttrs := map[string]any{}
	var prefix string
	var ip string

	if srcIp != "" {
		prefix = "source"
		ip = srcIp
	}

	if dstIp != "" {
		prefix = "destination"
		ip = dstIp
	}

	if prefix != "" {
		if pod := client.GetPodByIPAddr(ip); pod != nil {
			k8sEventAttrs[fmt.Sprintf("%s.%s", prefix, semconv.K8SPodNameKey)] = pod.Name
			k8sEventAttrs[fmt.Sprintf("%s.%s", prefix, semconv.K8SPodUIDKey)] = pod.UID
			k8sEventAttrs[fmt.Sprintf("%s.%s", prefix, semconv.K8SNamespaceNameKey)] = pod.Namespace

			if len(pod.Spec.Containers) > 0 {
				var containerNames []string
				for _, container := range pod.Spec.Containers {
					containerNames = append(containerNames, container.Name)
				}
				k8sEventAttrs[fmt.Sprintf("%s.%s", prefix, semconv.K8SContainerNameKey)] = strings.Join(containerNames, ",")
			}

			if node := client.GetNodeByPod(pod); node != nil {
				k8sEventAttrs[fmt.Sprintf("%s.%s", prefix, semconv.K8SNodeNameKey)] = node.Name
				k8sEventAttrs[fmt.Sprintf("%s.%s", prefix, semconv.K8SNodeUIDKey)] = node.UID
			}

			if service := client.GetServiceForPod(pod); service != nil {
				// no semconv for service yet
				k8sEventAttrs[fmt.Sprintf("%s.k8s.service.name", prefix)] = service.Name
			}
		}
	}

	return k8sEventAttrs
}
