package main

import (
	"log"
	"os"

	"github.com/honeycombio/ebpf-agent/bpf/socket"
	"github.com/honeycombio/libhoney-go"
)

func main() {
	// setup libhoney
	libhoney.Init(libhoney.Config{
		APIKey: os.Getenv("HONEYCOMB_API_KEY"),
		Dataset: os.Getenv("HONEYCOMB_DATASET"),
	})
	defer libhoney.Close()

	log.Println("Starting Honeypot eBPF Agent")
	// probes.Setup()
	socket.Setup()
}
