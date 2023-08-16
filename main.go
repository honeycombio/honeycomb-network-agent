package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/honeycombio/ebpf-agent/assemblers"
	"github.com/honeycombio/ebpf-agent/bpf/probes"
	"github.com/honeycombio/ebpf-agent/utils"
	"github.com/honeycombio/libhoney-go"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
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
	assember := assemblers.NewTcpAssembler(*agentConfig)
	go assember.Start()
	defer assember.Stop()

	log.Info().Msg("Agent is ready!")

	signalChannel := make(chan os.Signal, 1)
	signal.Notify(signalChannel, os.Interrupt, syscall.SIGTERM)
	<-signalChannel

	log.Info().Msg("Shutting down...")
}

func getEnvOrDefault(key string, defaultValue string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return defaultValue
}
