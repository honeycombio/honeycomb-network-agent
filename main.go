package main

import (
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"syscall"
	"time"

	"github.com/honeycombio/ebpf-agent/assemblers"
	"github.com/honeycombio/ebpf-agent/bpf/probes"
	"github.com/honeycombio/ebpf-agent/utils"
	"github.com/honeycombio/libhoney-go"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	semconv "go.opentelemetry.io/otel/semconv/v1.20.0"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

const Version string = "0.0.3-alpha"
const defaultDataset = "hny-ebpf-agent"
const defaultEndpoint = "https://api.honeycomb.io"

func main() {
	// Set logging level
	zerolog.SetGlobalLevel(zerolog.InfoLevel)
	if os.Getenv("DEBUG") == "true" {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	}

	log.Info().Str("agent_version", Version).Msg("Starting Honeycomb eBPF agent")

	kernelVersion, err := utils.HostKernelVersion()
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to get host kernel version")
	}
	btfEnabled := utils.HostBtfEnabled()
	log.Info().
		Str("kernel_version", kernelVersion.String()).
		Bool("btf_enabled", btfEnabled).
		Msg("Detected host kernel")

	apikey := os.Getenv("HONEYCOMB_API_KEY")
	if apikey == "" {
		log.Fatal().Msg("Honeycomb API key not set, unable to send events\n")
	}

	dataset := getEnvOrDefault("HONEYCOMB_DATASET", defaultDataset)
	endpoint := getEnvOrDefault("HONEYCOMB_API_ENDPOINT", defaultEndpoint)
	log.Info().
		Str("hny_endpoint", endpoint).
		Str("hny_dataset", dataset).
		Msg("Honeycomb API config")

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

	log.Info().Msg("Agent is ready!")

	signalChannel := make(chan os.Signal, 1)
	signal.Notify(signalChannel, os.Interrupt, syscall.SIGTERM)
	<-signalChannel

	log.Info().Msg("Shutting down...")
}

func handleHttpEvents(events chan assemblers.HttpEvent, client *kubernetes.Clientset) {
	ticker := time.NewTicker(time.Second * 10)
	for {
		select {
		case event := <-events:
			sendHttpEventToHoneycomb(event, client)
		case <-ticker.C:
			log.Info().
				Int("event queue length", len(events)).
				Int("goroutines", runtime.NumGoroutine()).
				Msg("Queue length ticker")
		}
	}
}

func sendHttpEventToHoneycomb(event assemblers.HttpEvent, client *kubernetes.Clientset) {
	// create libhoney event
	ev := libhoney.NewEvent()

	// common attributes
	ev.Timestamp = event.Timestamp
	ev.AddField("httpEvent_handled_at", time.Now())
	ev.AddField("httpEvent_handled_latency", time.Now().Sub(event.Timestamp))
	ev.AddField("goroutine_count", runtime.NumGoroutine())
	ev.AddField("duration_ms", event.Duration.Microseconds())
	ev.AddField(string(semconv.NetSockHostAddrKey), event.SrcIp)
	ev.AddField("destination.address", event.DstIp)

	// request attributes
	if event.Request != nil {
		bodySizeString := event.Request.Header.Get("Content-Length")
		bodySize, _ := strconv.ParseInt(bodySizeString, 10, 64)
		ev.AddField("name", fmt.Sprintf("HTTP %s", event.Request.Method))
		ev.AddField(string(semconv.HTTPMethodKey), event.Request.Method)
		ev.AddField(string(semconv.HTTPURLKey), event.Request.RequestURI)
		ev.AddField("http.request.body", fmt.Sprintf("%v", event.Request.Body))
		ev.AddField("http.request.headers", fmt.Sprintf("%v", event.Request.Header))
		ev.AddField(string(semconv.UserAgentOriginalKey), event.Request.Header.Get("User-Agent"))
		ev.AddField("http.request.body.size", bodySize)
	} else {
		ev.AddField("name", "HTTP")
		ev.AddField("http.request.missing", "no request on this event")
	}

	// response attributes
	if event.Response != nil {
		bodySizeString := event.Response.Header.Get("Content-Length")
		bodySize, _ := strconv.ParseInt(bodySizeString, 10, 64)

		ev.AddField(string(semconv.HTTPStatusCodeKey), event.Response.StatusCode)
		ev.AddField("http.response.body", event.Response.Body)
		ev.AddField("http.response.headers", event.Response.Header)
		ev.AddField("http.response.body.size", bodySize)

	} else {
		ev.AddField("http.response.missing", "no response on this event")
	}

	// k8s attributes
	// TODO: make this faster; the call to the k8s API takes a bit of time and
	//       slows the processing of the event queue
	// k8sEventAttrs := utils.GetK8sEventAttrs(client, event.SrcIp, event.DstIp)
	// ev.Add(k8sEventAttrs)

	log.Info().
		Time("event.timestamp", ev.Timestamp).
		Str("http.url", event.Request.RequestURI).
		Msg("Event sent")
	err := ev.Send()
	if err != nil {
		log.Debug().
			Err(err).
			Msg("error sending event")
	}
}

func getEnvOrDefault(key string, defaultValue string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return defaultValue
}
