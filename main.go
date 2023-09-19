package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/honeycombio/honeycomb-network-agent/assemblers"
	"github.com/honeycombio/honeycomb-network-agent/config"
	"github.com/honeycombio/honeycomb-network-agent/debug"
	"github.com/honeycombio/honeycomb-network-agent/utils"
	"github.com/honeycombio/libhoney-go"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	semconv "go.opentelemetry.io/otel/semconv/v1.20.0"
)

const Version string = "0.0.16-alpha"

func main() {
	config := config.NewConfig()
	if err := config.Validate(); err != nil {
		log.Fatal().Err(err).Msg("Config validation failed")
	}

	// setup logging first
	setupLogging(config)

	log.Info().
		Str("agent_version", Version).
		Str("api_key", config.GetMaskedAPIKey()).
		Str("endpoint", config.Endpoint).
		Str("dataset", config.Dataset).
		Str("stats_dataset", config.StatsDataset).
		Msg("Starting Honeycomb Network Agent")
	if config.Debug {
		log.Info().
			Str("debug_address", config.DebugAddress).
			Msg("Debug service enabled")
		// enable debug service
		debug := &debug.DebugService{Config: config}
		debug.Start()
	}

	// setup libhoney
	closeLibhoney := setupLibhoney(config)
	defer closeLibhoney()

	// setup k8s
	cachedK8sClient, closeK8s := setupK8s(config)
	defer closeK8s()

	// setup packet capture and analysis
	// TODO: Move event handling into the assembler
	httpEvents := make(chan assemblers.HttpEvent, config.ChannelBufferSize)
	assembler := assemblers.NewTcpAssembler(config, httpEvents)
	go handleHttpEvents(httpEvents, cachedK8sClient)
	go assembler.Start()
	defer assembler.Stop()

	log.Info().Msg("Agent is ready!")

	signalChannel := make(chan os.Signal, 1)
	signal.Notify(signalChannel, os.Interrupt, syscall.SIGTERM)
	<-signalChannel

	log.Info().Msg("Shutting down...")
}

func handleHttpEvents(events chan assemblers.HttpEvent, client *utils.CachedK8sClient) {
	for {
		event := <-events
		sendHttpEventToHoneycomb(event, client)
	}
}

func sendHttpEventToHoneycomb(event assemblers.HttpEvent, k8sClient *utils.CachedK8sClient) {
	// create libhoney event
	ev := libhoney.NewEvent()

	// calculate event duration
	// TODO: This is a hack to work around a bug that results in the response timestamp sometimes
	// being zero which causes the event duration to be negative.
	if event.RequestTimestamp.IsZero() {
		log.Debug().
			Uint64("stream_id", event.StreamId).
			Int64("request_id", event.RequestId).
			Msg("Request timestamp is zero")
		ev.AddField("http.request.timestamp_missing", true)
		event.RequestTimestamp = time.Now()
	}
	if event.ResponseTimestamp.IsZero() {
		log.Debug().
			Uint64("stream_id", event.StreamId).
			Int64("request_id", event.RequestId).
			Msg("Response timestamp is zero")
		ev.AddField("http.response.timestamp_missing", true)
		event.ResponseTimestamp = time.Now()
	}
	eventDuration := event.ResponseTimestamp.Sub(event.RequestTimestamp)

	// common attributes
	ev.Timestamp = event.RequestTimestamp
	ev.AddField("meta.httpEvent_handled_at", time.Now())
	ev.AddField("meta.httpEvent_request_handled_latency_ms", time.Since(event.RequestTimestamp).Milliseconds())
	ev.AddField("meta.httpEvent_response_handled_latency_ms", time.Since(event.ResponseTimestamp).Milliseconds())
	ev.AddField("duration_ms", eventDuration.Milliseconds())
	ev.AddField("http.request.timestamp", event.RequestTimestamp)
	ev.AddField("http.response.timestamp", event.ResponseTimestamp)
	ev.AddField("http.request.id", event.RequestId)

	ev.AddField(string(semconv.NetSockHostAddrKey), event.SrcIp)
	ev.AddField("destination.address", event.DstIp)

	var requestURI string

	// request attributes
	if event.Request != nil {
		requestURI = event.Request.RequestURI
		ev.AddField("name", fmt.Sprintf("HTTP %s", event.Request.Method))
		ev.AddField(string(semconv.HTTPMethodKey), event.Request.Method)
		ev.AddField(string(semconv.HTTPURLKey), requestURI)
		ev.AddField(string(semconv.UserAgentOriginalKey), event.Request.Header.Get("User-Agent"))
		ev.AddField("http.request.body.size", event.Request.ContentLength)
	} else {
		ev.AddField("name", "HTTP")
		ev.AddField("http.request.missing", "no request on this event")
	}

	// response attributes
	if event.Response != nil {
		ev.AddField(string(semconv.HTTPStatusCodeKey), event.Response.StatusCode)
		ev.AddField("http.response.body.size", event.Response.ContentLength)

	} else {
		ev.AddField("http.response.missing", "no response on this event")
	}

	k8sEventAttrs := utils.GetK8sEventAttrs(k8sClient, event.SrcIp, event.DstIp)
	ev.Add(k8sEventAttrs)

	log.Debug().
		Uint64("stream_id", event.StreamId).
		Int64("request_id", event.RequestId).
		Time("event.timestamp", ev.Timestamp).
		Str("http.url", requestURI).
		Msg("Event sent")
	err := ev.Send()
	if err != nil {
		log.Debug().
			Err(err).
			Msg("error sending event")
	}
}

// setupLogging initializes zerolog
func setupLogging(c config.Config) {
	level, err := zerolog.ParseLevel(c.LogLevel)
	if err != nil {
		log.Warn().Err(err).Str("log_level", c.LogLevel).Msg("Failed to parse log level, defaulting to INFO")
		level = zerolog.InfoLevel
	}
	zerolog.SetGlobalLevel(level)

	// enable pretty printing
	if level == zerolog.DebugLevel {
		log.Logger = log.Output(zerolog.NewConsoleWriter())
	}
}

// setupLibhoney initializes libhoney and sets global fields
func setupLibhoney(config config.Config) func() {
	libhoney.Init(libhoney.Config{
		APIKey:  config.APIKey,
		Dataset: config.Dataset,
		APIHost: config.Endpoint,
	})

	// appends libhoney's user-agent (TODO: doesn't work, no useragent right now)
	libhoney.UserAgentAddition = fmt.Sprintf("hny-network-agent/%s", Version)

	// configure global fields that are set on all events
	libhoney.AddField("honeycomb.agent_version", Version)

	return libhoney.Close
}

// setupK8s gets the k8s cluster config, creates a k8s clientset then creates and starts
// cached k8s client that caches k8s objects
func setupK8s(config config.Config) (*utils.CachedK8sClient, func()) {
	// get the k8s in-cluster config
	k8sConfig, err := rest.InClusterConfig()
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to get kubernetes cluster config")
	}

	// create k8s clientset
	k8sClient, err := kubernetes.NewForConfig(k8sConfig)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to create kubernetes client")
	}

	// create k8s monitor that caches k8s objects
	ctx, done := context.WithCancel(context.Background())
	cachedK8sClient := utils.NewCachedK8sClient(k8sClient)
	cachedK8sClient.Start(ctx)
	return cachedK8sClient, done
}
