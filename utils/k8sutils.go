package utils

import (
	"fmt"
	"strings"

	"github.com/rs/zerolog/log"
	semconv "go.opentelemetry.io/otel/semconv/v1.20.0"
)

func GetK8sEventAttrs(client *CachedK8sClient, srcIp string, dstIp string) map[string]any {

	dstPod := client.GetPodByIPAddr(dstIp)
	srcPod := client.GetPodByIPAddr(srcIp)
	srcNode := client.GetNodeByPod(srcPod)
	service := client.GetServiceForPod(srcPod)
	log.Info().
		Str("src_ip", srcIp).
		Str("dst_ip", dstIp).
		Msg("Getting k8s event attrs")
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
