package main

import (
	"fmt"
	"log"
	"os"

	"github.com/honeycombio/ebpf-agent/bpf/probes"
	"github.com/honeycombio/ebpf-agent/utils"
	"github.com/honeycombio/libhoney-go"
)

const defaultDataset = "hny-ebpf-agent"
const defaultEndpoint = "https://api.honeycomb.io"

func main() {
	agentVersion := Version()
	log.Printf("Starting Honeycomb eBPF agent v%s\n", agentVersion)

	kernelVersion, err := utils.HostKernelVersion()
	if err != nil {
		log.Fatalf("Failed to get host kernel version: %v", err)
	}
	log.Printf("Host kernel version: %s\n", kernelVersion)

	btfEnabled := utils.HostBtfEnabled()
	log.Printf("BTF enabled: %s\n", btfEnabled)

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
	libhoney.UserAgentAddition = fmt.Sprintf("hny/ebpf-agent/%s", agentVersion)

	// add agent and kernel version fields to all libhoney events
	libhoney.AddField("honeycomb.agent_version", agentVersion)
	libhoney.AddField("meta.kernel_version", kernelVersion)
	libhoney.AddField("meta.btf_enabled", btfEnabled)

	defer libhoney.Close()

	// setup probes
	probes.Setup()
}

func getEnvOrDefault(key string, defaultValue string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return defaultValue
}
