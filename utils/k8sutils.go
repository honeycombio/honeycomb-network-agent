package utils

import (
	"fmt"
	"strings"

	"go.opentelemetry.io/otel/attribute"
	semconv "go.opentelemetry.io/otel/semconv/v1.20.0"
	"go.opentelemetry.io/otel/trace"
)

func AddK8sAttrsToSpan(span trace.Span, client *CachedK8sClient, srcIp string, dstIp string) {
	if srcPod := client.GetPodByIPAddr(srcIp); srcPod != nil {
		span.SetAttributes(
			semconv.K8SPodName(srcPod.Name),
			semconv.K8SPodUID(fmt.Sprint(srcPod.UID)),
			semconv.K8SNamespaceName(srcPod.Namespace),
		)

		if len(srcPod.Spec.Containers) > 0 {
			var containerNames []string
			for _, container := range srcPod.Spec.Containers {
				containerNames = append(containerNames, container.Name)
			}
			span.SetAttributes(
				semconv.K8SContainerName(strings.Join(containerNames, ",")),
			)
		}

		if srcNode := client.GetNodeByPod(srcPod); srcNode != nil {
			span.SetAttributes(
				semconv.K8SNodeName(srcNode.Name),
				semconv.K8SNodeUID(fmt.Sprint(srcNode.UID)),
			)
		}

		if service := client.GetServiceForPod(srcPod); service != nil {
			span.SetAttributes(
				attribute.String("k8s.service.name", service.Name),
			)
		}
	}

	if dstPod := client.GetPodByIPAddr(dstIp); dstPod != nil {
		span.SetAttributes(
			attribute.String(fmt.Sprintf("destination.%s", semconv.K8SPodNameKey), dstPod.Name),
			attribute.String(fmt.Sprintf("destination.%s", semconv.K8SPodUIDKey), fmt.Sprint(dstPod.UID)),
		)
	}
}
