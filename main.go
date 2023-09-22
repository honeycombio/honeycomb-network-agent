package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/honeycombio/honeycomb-network-agent/assemblers"
	"github.com/honeycombio/honeycomb-network-agent/config"
	"github.com/honeycombio/honeycomb-network-agent/debug"
	"github.com/honeycombio/honeycomb-network-agent/handlers"
	"github.com/honeycombio/honeycomb-network-agent/utils"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

const Version string = "0.0.17-alpha"

func main() {
	config := config.NewConfig()
	if err := config.Validate(); err != nil {
		log.Fatal().Err(err).Msg("Config validation failed")
	}

	// setup logging first
	// TODO: move to utils package?
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

	// setup context and cancel func used to signal shutdown
	ctx, done := context.WithCancel(context.Background())

	// setup k8s
	// TODO: move setupK8s to utils package?
	cachedK8sClient := setupK8s(ctx, config)

	// create events channel for assembler to send events to and event handler to receive events from
	eventsChannel := make(chan assemblers.HttpEvent, config.ChannelBufferSize)

	// create event handler that sends events to backend (eg Honeycomb)
	// TODO: move version outside of main package so it can be used directly in the eventHandler
	eventHandler := handlers.NewLibhoneyEventHandler(config, cachedK8sClient, eventsChannel, Version)
	go eventHandler.Start(ctx)

	// create assembler that does packet capture and analysis
	assembler := assemblers.NewTcpAssembler(config, eventsChannel)
	go assembler.Start(ctx)

	// listen for shutdown and interrupt signals
	signalChannel := make(chan os.Signal, 1)
	signal.Notify(signalChannel, os.Interrupt, syscall.SIGTERM)
	shutdownChannel := make(chan bool, 1)

	// cleanup func that waits for for shutdown signal and stops the assembler, event handler and k8s client
	go func() {
		<-signalChannel
		log.Info().Msg("Agent is stopping. Cleaning up...")
		// signal the k8sClient, event handler and assembler to stop via ctx.Done()
		done()
		<-shutdownChannel
	}()

	log.Info().Msg("Agent is ready!")
	<-shutdownChannel
	log.Info().Msg("Agent has stopped")
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

// setupK8s gets the k8s cluster config, creates a k8s clientset then creates and starts
// cached k8s client that caches k8s objects
func setupK8s(ctx context.Context, config config.Config) *utils.CachedK8sClient {
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
	cachedK8sClient := utils.NewCachedK8sClient(k8sClient)
	cachedK8sClient.Start(ctx)
	return cachedK8sClient
}
