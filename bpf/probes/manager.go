package probes

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"

	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/perf"
	"github.com/honeycombio/libhoney-go"
	"github.com/rs/zerolog/log"
	semconv "go.opentelemetry.io/otel/semconv/v1.20.0"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
)

//go:generate go run github.com/cilium/ebpf/cmd/bpf2go -target amd64,arm64 -cc clang -cflags $CFLAGS bpf source/tcp_probe.c

const mapKey uint32 = 0

type manager struct {
	bpfObjects bpfObjects
	probes     []link.Link
	reader     *perf.Reader
	client     *kubernetes.Clientset
}

func New(client *kubernetes.Clientset) manager {
	// Load pre-compiled programs and maps into the kernel.
	objs := bpfObjects{}
	if err := loadBpfObjects(&objs, nil); err != nil {
		log.Fatal().Err(err).Msg("failed loading objects")
	}
	defer objs.Close()

	// Deploy tcp_connect kprobe
	kprobeTcpConnect, err := link.Kprobe("tcp_connect", objs.KprobeTcpConnect, nil)
	if err != nil {
		log.Fatal().Err(err).Msg("failed opening kprobe")
	}
	defer kprobeTcpConnect.Close()

	// Deploy tcp_close kprobe
	kprobeTcpClose, err := link.Kprobe("tcp_close", objs.KprobeTcpClose, nil)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to open kretprobe")
	}
	defer kprobeTcpClose.Close()

	// Setup perf event reader to read probe events
	reader, err := perf.NewReader(objs.Events, os.Getpagesize())
	if err != nil {
		log.Fatal().Err(err).Msg("failed creating perf reader")
	}

	return manager{
		bpfObjects: objs,
		probes:     []link.Link{kprobeTcpConnect, kprobeTcpClose},
		reader:     reader,
		client:     client,
	}
}

func (m *manager) Start() {
	// bpfTcpEvent is generated by bpf2go from tcp_event struct in tcp_probe.c
	var event bpfTcpEvent
	for {
		record, err := m.reader.Read()
		if err != nil {
			if errors.Is(err, perf.ErrClosed) {
				return
			}
			continue
		}

		if record.LostSamples != 0 {
			continue
		}

		if err := binary.Read(bytes.NewBuffer(record.RawSample), binary.LittleEndian, &event); err != nil {
			log.Error().Err(err).Msg("error parsing perf event")
			continue
		}

		// log.Printf("event: %+v\n", event)

		sendEvent(event, m.client)
	}
}

func (m *manager) Stop() {
	for _, probe := range m.probes {
		probe.Close()
	}
	m.bpfObjects.Close()
	m.reader.Close()
}

func getPodByIPAddr(client *kubernetes.Clientset, ipAddr string) v1.Pod {
	pods, _ := client.CoreV1().Pods(v1.NamespaceAll).List(context.TODO(), metav1.ListOptions{})

	var matchedPod v1.Pod

	for _, pod := range pods.Items {
		if ipAddr == pod.Status.PodIP {
			matchedPod = pod
		}
	}

	return matchedPod
}

func getServiceForPod(client *kubernetes.Clientset, inputPod v1.Pod) v1.Service {
	// get list of services
	services, _ := client.CoreV1().Services(v1.NamespaceAll).List(context.TODO(), metav1.ListOptions{})
	var matchedService v1.Service
	// loop over services
	for _, service := range services.Items {
		set := labels.Set(service.Spec.Selector)
		listOptions := metav1.ListOptions{LabelSelector: set.AsSelector().String()}
		pods, err := client.CoreV1().Pods(v1.NamespaceAll).List(context.TODO(), listOptions)
		if err != nil {
			log.Error().Err(err).Msg("Error getting pods")
		}
		for _, pod := range pods.Items {
			if pod.Name == inputPod.Name {
				matchedService = service
			}
		}
	}

	return matchedService
}

func getNodeByPod(client *kubernetes.Clientset, pod v1.Pod) *v1.Node {
	node, _ := client.CoreV1().Nodes().Get(context.TODO(), pod.Spec.NodeName, metav1.GetOptions{})
	return node
}

// Send event to Honeycomb
func sendEvent(event bpfTcpEvent, client *kubernetes.Clientset) {

	sourceIpAddr := intToIP(event.Saddr).String()
	destIpAddr := intToIP(event.Daddr).String()

	destPod := getPodByIPAddr(client, destIpAddr)
	sourcePod := getPodByIPAddr(client, sourceIpAddr)
	sourceNode := getNodeByPod(client, sourcePod)

	ev := libhoney.NewEvent()
	ev.AddField("name", "tcp_event")
	ev.AddField("duration_ms", (event.EndTime-event.StartTime)/1_000_000) // convert ns to ms
	// IP Address / port
	ev.AddField(string(semconv.NetSockHostAddrKey), sourceIpAddr)
	ev.AddField("destination.address", destIpAddr)
	ev.AddField(string(semconv.NetHostPortKey), event.Sport)
	ev.AddField("destination.port", event.Dport)

	// dest pod
	ev.AddField(fmt.Sprintf("destination.%s", semconv.K8SPodNameKey), destPod.Name)
	ev.AddField(fmt.Sprintf("destination.%s", semconv.K8SPodUIDKey), destPod.UID)

	// source pod
	ev.AddField(string(semconv.K8SPodNameKey), sourcePod.Name)
	ev.AddField(string(semconv.K8SPodUIDKey), sourcePod.UID)

	// namespace
	ev.AddField(string(semconv.K8SNamespaceNameKey), sourcePod.Namespace)

	// service
	// no semconv for service yet
	ev.AddField("k8s.service.name", getServiceForPod(client, sourcePod).Name)

	// node
	ev.AddField(string(semconv.K8SNodeNameKey), sourceNode.Name)
	ev.AddField(string(semconv.K8SNodeUIDKey), sourceNode.UID)

	// container names
	if len(sourcePod.Spec.Containers) > 0 {
		var containerNames []string
		for _, container := range sourcePod.Spec.Containers {
			containerNames = append(containerNames, container.Name)
		}
		ev.AddField(string(semconv.K8SContainerNameKey), strings.Join(containerNames, ","))
	}

	err := ev.Send()
	if err != nil {
		log.Printf("error sending event: %v\n", err)
	}
}

// intToIP converts IPv4 number to net.IP
func intToIP(ipNum uint32) net.IP {
	ip := make(net.IP, 4)
	binary.LittleEndian.PutUint32(ip, ipNum)
	return ip
}
