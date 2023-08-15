package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/honeycombio/ebpf-agent/assemblers"
	"github.com/honeycombio/ebpf-agent/bpf/probes"
	"github.com/honeycombio/ebpf-agent/utils"
	"github.com/honeycombio/libhoney-go"
	semconv "go.opentelemetry.io/otel/semconv/v1.20.0"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

const Version string = "0.0.3-alpha"
const defaultDataset = "hny-ebpf-agent"
const defaultEndpoint = "https://api.honeycomb.io"

func main() {
	log.Printf("Starting Honeycomb eBPF agent v%s\n", Version)

	kernelVersion, err := utils.HostKernelVersion()
	if err != nil {
		log.Fatalf("Failed to get host kernel version: %v", err)
	}
	log.Printf("Host kernel version: %s\n", kernelVersion)

	btfEnabled := utils.HostBtfEnabled()
	log.Printf("BTF enabled: %v\n", btfEnabled)

	apikey := os.Getenv("HONEYCOMB_API_KEY")
	if apikey == "" {
		log.Fatalf("Honeycomb API key not set, unable to send events\n")
	}

	dataset := getEnvOrDefault("HONEYCOMB_DATASET", defaultDataset)
	log.Printf("Honeycomb dataset: %s\n", dataset)

	endpoint := getEnvOrDefault("HONEYCOMB_API_ENDPOINT", defaultEndpoint)
	log.Printf("Honeycomb API endpoint: %s\n", endpoint)

	// setup libhoney
	libhoney.Init(libhoney.Config{
		APIKey:  apikey,
		Dataset: dataset,
		APIHost: endpoint,
	})

	// appends libhoney's user-agent (TODO: doesn't work, no useragent right now)
	libhoney.UserAgentAddition = fmt.Sprintf("hny/ebpf-agent/%s", Version)

	// configure global fields that are set on all events
	libhoney.AddField("honeycomb.agent_version", Version)
	libhoney.AddField("meta.kernel_version", kernelVersion.String())
	libhoney.AddField("meta.btf_enabled", btfEnabled)

	defer libhoney.Close()

	// creates the in-cluster config
	k8sConfig, err := rest.InClusterConfig()
	if err != nil {
		panic(err.Error())
	}

	// creates the clientset
	k8sClient, err := kubernetes.NewForConfig(k8sConfig)

	if err != nil {
		panic(err.Error())
	}

	// setup probes
	p := probes.New(k8sClient)
	go p.Start()
	defer p.Stop()

	agentConfig := assemblers.NewConfig()

	// setup TCP stream reader
	httpEvents := make(chan assemblers.HttpEvent, 10000)
	assember := assemblers.NewTcpAssembler(*agentConfig, httpEvents)
	go handleHttpEvents(httpEvents, k8sClient)
	go assember.Start()
	defer assember.Stop()

	log.Println("Agent is ready!")

	signalChannel := make(chan os.Signal, 1)
	signal.Notify(signalChannel, os.Interrupt, syscall.SIGTERM)
	<-signalChannel

	log.Println("Shutting down...")
}

func handleHttpEvents(events chan assemblers.HttpEvent, client *kubernetes.Clientset) {
	for {
		select {
		case event := <-events:

			dstPod := getPodByIPAddr(client, event.DstIp)
			srcPod := getPodByIPAddr(client, event.SrcIp)
			srcNode := getNodeByPod(client, srcPod)

			ev := libhoney.NewEvent()
			ev.AddField("duration_ms", event.Duration.Microseconds())
			ev.AddField("http.source_ip", event.SrcIp)
			ev.AddField("http.destination_ip", event.DstIp)
			if event.Request != nil {
				ev.AddField("name", fmt.Sprintf("HTTP %s", event.Request.Method))
				ev.AddField(string(semconv.HTTPMethodKey), event.Request.Method)
				ev.AddField(string(semconv.HTTPURLKey), event.Request.RequestURI)
				ev.AddField("http.request.body", fmt.Sprintf("%v", event.Request.Body))
				ev.AddField("http.request.headers", fmt.Sprintf("%v", event.Request.Header))
			} else {
				ev.AddField("name", "HTTP")
				ev.AddField("http.request.missing", "no request on this event")
			}

			if event.Response != nil {
				ev.AddField(string(semconv.HTTPStatusCodeKey), event.Response.StatusCode)
				ev.AddField("http.response.body", event.Response.Body)
				ev.AddField("http.response.headers", event.Response.Header)
			} else {
				ev.AddField("http.response.missing", "no response on this event")
			}

			// dest pod
			ev.AddField(fmt.Sprintf("destination.%s", semconv.K8SPodNameKey), dstPod.Name)
			ev.AddField(fmt.Sprintf("destination.%s", semconv.K8SPodUIDKey), dstPod.UID)

			// source pod
			ev.AddField(string(semconv.K8SPodNameKey), srcPod.Name)
			ev.AddField(string(semconv.K8SPodUIDKey), srcPod.UID)

			// namespace
			ev.AddField(string(semconv.K8SNamespaceNameKey), srcPod.Namespace)

			// service
			// no semconv for service yet
			ev.AddField("k8s.service.name", getServiceForPod(client, srcPod).Name)

			// node
			ev.AddField(string(semconv.K8SNodeNameKey), srcNode.Name)
			ev.AddField(string(semconv.K8SNodeUIDKey), srcNode.UID)

			// container names
			if len(srcPod.Spec.Containers) > 0 {
				var containerNames []string
				for _, container := range srcPod.Spec.Containers {
					containerNames = append(containerNames, container.Name)
				}
				ev.AddField(string(semconv.K8SContainerNameKey), strings.Join(containerNames, ","))
			}

			//TODO: Body size produces a runtime error, commenting out for now.
			// requestSize := getBodySize(event.request.Body)
			// ev.AddField("http.request.body.size", requestSize)
			// responseSize := getBodySize(event.response.Body)
			// ev.AddField("http.response.body.size", responseSize)

			err := ev.Send()
			if err != nil {
				log.Printf("error sending event: %v\n", err)
			}
		}
	}
}

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

func getBodySize(r io.ReadCloser) int {
	length := 0
	b, err := io.ReadAll(r)
	if err == nil {
		length = len(b)
		r.Close()
	}

	return length
}

func getEnvOrDefault(key string, defaultValue string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return defaultValue
}
