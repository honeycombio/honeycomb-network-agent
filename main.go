package main

import (
	"log"
	"os"

	"github.com/honeycombio/ebpf-agent/bpf/probes"
	"github.com/honeycombio/ebpf-agent/utils"
	"github.com/honeycombio/libhoney-go"
)

const Version string = "0.0.1"

func main() {
	log.Printf("Starting Honeycomb eBPF agent v%s\n", Version)

	// Try to detect host kernel kernelVersion
	kernelVersion, err := utils.HostKernelVersion()
	if err != nil {
		log.Fatalf("Failed to get host kernel version: %v", err)
	}
	log.Printf("Host kernel version: %s\n", kernelVersion)
	log.Printf("Host kernel version: %s\n", os.Getenv("HONEYCOMB_API_ENDPOINT"))
	// setup libhoney
	libhoney.Init(libhoney.Config{
		APIKey:  os.Getenv("HONEYCOMB_API_KEY"),
		Dataset: os.Getenv("HONEYCOMB_DATASET"),
		APIHost: os.Getenv("HONEYCOMB_API_ENDPOINT"),
	})
	defer libhoney.Close()

	probes.Setup()
}
