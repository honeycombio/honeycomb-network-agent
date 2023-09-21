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

	if srcPod := client.GetPodByIPAddr(srcIp); srcPod != nil {
		k8sEventAttrs[string(semconv.K8SPodNameKey)] = srcPod.Name
		k8sEventAttrs[string(semconv.K8SPodUIDKey)] = srcPod.UID
		k8sEventAttrs[string(semconv.K8SNamespaceNameKey)] = srcPod.Namespace

		if len(srcPod.Spec.Containers) > 0 {
			var containerNames []string
			for _, container := range srcPod.Spec.Containers {
				containerNames = append(containerNames, container.Name)
			}
			k8sEventAttrs[string(semconv.K8SContainerNameKey)] = strings.Join(containerNames, ",")
		}

		if srcNode := client.GetNodeByPod(srcPod); srcNode != nil {
			k8sEventAttrs[string(semconv.K8SNodeNameKey)] = srcNode.Name
			k8sEventAttrs[string(semconv.K8SNodeUIDKey)] = srcNode.UID
		}

		if service := client.GetServiceForPod(srcPod); service != nil {
			// no semconv for service yet
			k8sEventAttrs["k8s.service.name"] = service.Name
		}
	}

	if dstPod := client.GetPodByIPAddr(dstIp); dstPod != nil {
		k8sEventAttrs[fmt.Sprintf("destination.%s", semconv.K8SPodNameKey)] = dstPod.Name
		k8sEventAttrs[fmt.Sprintf("destination.%s", semconv.K8SPodUIDKey)] = dstPod.UID
	}

	return k8sEventAttrs
}
