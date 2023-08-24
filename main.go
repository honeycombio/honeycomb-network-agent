package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"syscall"
	"time"

	"github.com/honeycombio/ebpf-agent/assemblers"
	"github.com/honeycombio/ebpf-agent/utils"
	"github.com/honeycombio/otel-config-go/otelconfig"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	semconv "go.opentelemetry.io/otel/semconv/v1.20.0"
	"go.opentelemetry.io/otel/trace"
)

const Name string = "hny-ebpf-agent"
const Version string = "0.0.5-alpha"
const defaultDataset = "hny-ebpf-agent"
const defaultEndpoint = "api.honeycomb.io:443"

func main() {
	// Set logging level
	zerolog.SetGlobalLevel(zerolog.InfoLevel)
	if os.Getenv("DEBUG") == "true" {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	}
	// TODO: add a flag to enable human readable logs
	// log.Logger = log.Output(zerolog.NewConsoleWriter())

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

	otelShutdown, err := otelconfig.ConfigureOpenTelemetry(
		otelconfig.WithServiceName(Name),
		otelconfig.WithServiceVersion(Version),
		otelconfig.WithExporterEndpoint(endpoint),
		otelconfig.WithHeaders(map[string]string{
			"x-honeycomb-team":    apikey,
			"x-honeycomb-dataset": dataset,
		}),
		otelconfig.WithResourceAttributes(map[string]string{
			"honeycomb.agent_version": Version,
			"meta.kernel_version":     kernelVersion.String(),
			"meta.btf_enabled":        strconv.FormatBool(btfEnabled),
		}),
	)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to configure OpenTelemetry")
	}
	defer otelShutdown()

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

	// create k8s monitor that caches k8s objects
	ctx, done := context.WithCancel(context.Background())
	defer done()
	cachedK8sClient := utils.NewCachedK8sClient(k8sClient)
	cachedK8sClient.Start(ctx)

	agentConfig := assemblers.NewConfig()

	// setup TCP stream reader
	httpEvents := make(chan assemblers.HttpEvent, 10000)
	assember := assemblers.NewTcpAssembler(*agentConfig, httpEvents)
	go handleHttpEvents(httpEvents, cachedK8sClient)
	go assember.Start()
	defer assember.Stop()

	log.Info().Msg("Agent is ready!")

	signalChannel := make(chan os.Signal, 1)
	signal.Notify(signalChannel, os.Interrupt, syscall.SIGTERM)
	<-signalChannel

	log.Info().Msg("Shutting down...")
}

func handleHttpEvents(events chan assemblers.HttpEvent, client *utils.CachedK8sClient) {
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

func sendHttpEventToHoneycomb(event assemblers.HttpEvent, k8sClient *utils.CachedK8sClient) {
	// start new span and set start time from event
	_, span := otel.Tracer(Name).
		Start(context.Background(), fmt.Sprintf("HTTP %s", event.Request.Method),
			trace.WithTimestamp(event.Start),
			trace.WithAttributes(
				attribute.String("httpEvent_handled_at", time.Now().String()),
				attribute.Int64("httpEvent_handled_latency_ms", event.Duration.Milliseconds()),
				attribute.Int("goroutine_count", runtime.NumGoroutine()),
				attribute.Int64("duration_ms", event.Duration.Milliseconds()),
				semconv.NetSockHostAddr(event.SrcIp),
				attribute.String("destination.address", event.DstIp),
			),
		)

	var requestURI string

	// request attributes
	if event.Request != nil {
		requestURI = event.Request.RequestURI
		bodySizeString := event.Request.Header.Get("Content-Length")
		bodySize, _ := strconv.ParseInt(bodySizeString, 10, 64)
		span.SetAttributes(
			attribute.String("name", fmt.Sprintf("HTTP %s", event.Request.Method)),
			semconv.HTTPMethod(event.Request.Method),
			semconv.HTTPURL(requestURI),
			semconv.UserAgentOriginal(event.Request.Header.Get("User-Agent")),
			attribute.Int64("http.request.body.size", bodySize),
		)
	} else {
		span.SetAttributes(
			attribute.String("name", "HTTP"),
			attribute.String("http.request.missing", "no request on this event"),
		)
	}

	// response attributes
	if event.Response != nil {
		bodySizeString := event.Response.Header.Get("Content-Length")
		bodySize, _ := strconv.ParseInt(bodySizeString, 10, 64)
		span.SetAttributes(
			semconv.HTTPStatusCode(event.Response.StatusCode),
			attribute.Int64("http.response.body.size", bodySize),
		)

	} else {
		span.SetAttributes(
			attribute.String("http.response.missing", "no response on this event"),
		)
	}

	// TODO: mayne move k8s attrs here?
	utils.AddK8sAttrsToSpan(span, k8sClient, event.SrcIp, event.DstIp)

	log.Debug().
		Str("request_id", event.RequestId).
		Time("event.timestamp", event.Start).
		Str("http.url", requestURI).
		Msg("Event sent")

	// end span and set end time -- also marks span ready for dispatch
	span.End(trace.WithTimestamp(event.End))
}

func getEnvOrDefault(key string, defaultValue string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return defaultValue
}
