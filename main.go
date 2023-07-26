package main

import (
	"context"
	"log"
	"os"

	"github.com/honeycombio/ebpf-agent/bpf/probes"
	"github.com/honeycombio/ebpf-agent/utils"
	"github.com/honeycombio/libhoney-go"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

const Version string = "0.0.1"
const defaultDataset = "hny-ebpf-agent"
const defaultEndpoint = "https://api.honeycomb.io"

func getKubernetesClient() *kubernetes.Clientset {
	// creates the in-cluster config
	config, err := rest.InClusterConfig()
	if err != nil {
		panic(err.Error())
	}
	// creates the clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}

	return clientset
}

func getPodsMetadata(client *kubernetes.Clientset) {
	log.Println("Loading Pod Metadata...")
	services, err := client.CoreV1().Services(v1.NamespaceAll).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		panic(err.Error())
	}
	for _, v := range services.Items {
		log.Printf("Service %v", v)
	}
}

func main() {
	log.Printf("Starting Honeycomb eBPF agent v%s\n", Version)

	// Try to detect host kernel kernelVersion
	kernelVersion, err := utils.HostKernelVersion()
	if err != nil {
		log.Fatalf("Failed to get host kernel version: %v", err)
	}
	log.Printf("Host kernel version: %s\n", kernelVersion)

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
	defer libhoney.Close()

	// setup pod metadata
	client := getKubernetesClient()
	getPodsMetadata(client)

	// setup probes
	probes.Setup()
}

func getEnvOrDefault(key string, defaultValue string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return defaultValue
}
