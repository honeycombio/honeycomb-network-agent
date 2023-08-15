package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/honeycombio/ebpf-agent/assemblers"
	"github.com/honeycombio/ebpf-agent/bpf/probes"
	"github.com/honeycombio/ebpf-agent/utils"
	"github.com/honeycombio/libhoney-go"
	semconv "go.opentelemetry.io/otel/semconv/v1.20.0"
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

			ev := libhoney.NewEvent()
			ev.AddField("duration_ms", event.Duration.Microseconds())
			ev.AddField(string(semconv.NetSockHostAddrKey), event.SrcIp)
			ev.AddField("destination.address", event.DstIp)
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

			// k8s metadata
			k8sEventAttrs := utils.GetK8sEventAttrs(client, event.SrcIp, event.DstIp)
			ev.Add(k8sEventAttrs)

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
