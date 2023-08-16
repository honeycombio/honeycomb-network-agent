package assemblers

import (
	"flag"
	"fmt"
	"io"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/ip4defrag"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"
	"github.com/google/gopacket/reassembly"
	"github.com/honeycombio/libhoney-go"
	semconv "go.opentelemetry.io/otel/semconv/v1.20.0"
)

var stats struct {
	ipdefrag            int
	missedBytes         int
	pkt                 int
	sz                  int
	totalsz             int
	rejectFsm           int
	rejectOpt           int
	rejectConnFsm       int
	reassembled         int
	outOfOrderBytes     int
	outOfOrderPackets   int
	biggestChunkBytes   int
	biggestChunkPackets int
	overlapBytes        int
	overlapPackets      int
}

var logLevel int
var errorsMap map[string]uint
var errorsMapMutex sync.Mutex
var errors uint

type Context struct {
	CaptureInfo gopacket.CaptureInfo
}

func (c *Context) GetCaptureInfo() gopacket.CaptureInfo {
	return c.CaptureInfo
}

type tcpAssembler struct {
	config        *config
	handle        *pcap.Handle
	packetSource  *gopacket.PacketSource
	streamFactory *tcpStreamFactory
	streamPool    *reassembly.StreamPool
	assembler     *reassembly.Assembler
	httpEvents    chan httpEvent
}

func NewTcpAssembler(config config) tcpAssembler {
	var handle *pcap.Handle
	var err error

	// Set logging level
	if *debug {
		logLevel = 2
	} else if *verbose {
		logLevel = 1
	} else if *quiet {
		logLevel = -1
	}

	errorsMap = make(map[string]uint)
	// Set up pcap packet capture
	if *fname != "" {
		log.Printf("Reading from pcap dump %q", *fname)
		handle, err = pcap.OpenOffline(*fname)
	} else {
		log.Printf("Starting capture on interface %q", *iface)
		handle, err = pcap.OpenLive(*iface, int32(*snaplen), true, pcap.BlockForever)
	}
	if err != nil {
		log.Fatal(err)
	}
	if len(flag.Args()) > 0 {
		bpffilter := strings.Join(flag.Args(), " ")
		Info("Using BPF filter %q\n", bpffilter)
		if err = handle.SetBPFFilter(bpffilter); err != nil {
			log.Fatal("BPF filter error:", err)
		}
	}

	packetSource := gopacket.NewPacketSource(handle, handle.LinkType())
	packetSource.Lazy = *lazy
	packetSource.NoCopy = true
	Info("Starting to read packets\n")

	httpEvents := make(chan httpEvent, 10000)
	streamFactory := NewTcpStreamFactory(httpEvents)
	streamPool := reassembly.NewStreamPool(&streamFactory)
	assembler := reassembly.NewAssembler(streamPool)

	return tcpAssembler{
		config:        &config,
		handle:        handle,
		packetSource:  packetSource,
		streamFactory: &streamFactory,
		streamPool:    streamPool,
		assembler:     assembler,
		httpEvents:    httpEvents,
	}
}

func (h *tcpAssembler) Start() {

	// start up http event handler
	// TODO: move this up to main.go level to acces k8s pod metadata
	go handleHttpEvents(h.httpEvents)

	count := 0
	bytes := int64(0)
	start := time.Now()
	defragger := ip4defrag.NewIPv4Defragmenter()
	for packet := range h.packetSource.Packets() {
		count++
		data := packet.Data()
		bytes += int64(len(data))
		// defrag the IPv4 packet if required
		if !h.config.Nodefrag {
			ip4Layer := packet.Layer(layers.LayerTypeIPv4)
			if ip4Layer == nil {
				continue
			}
			ip4 := ip4Layer.(*layers.IPv4)
			l := ip4.Length
			newip4, err := defragger.DefragIPv4(ip4)
			if err != nil {
				log.Fatalln("Error while de-fragmenting", err)
			} else if newip4 == nil {
				// Debug("Fragment...\n")
				log.Printf("Fragment...\n")
				continue // packet fragment, we don't have whole packet yet.
			}
			if newip4.Length != l {
				stats.ipdefrag++
				// Debug("Decoding re-assembled packet: %s\n", newip4.NextLayerType())
				log.Printf("Decoding re-assembled packet: %s\n", newip4.NextLayerType())
				pb, ok := packet.(gopacket.PacketBuilder)
				if !ok {
					panic("Not a PacketBuilder")
				}
				nextDecoder := newip4.NextLayerType()
				nextDecoder.Decode(newip4.Payload, pb)
			}
		}

		tcp := packet.Layer(layers.LayerTypeTCP)
		if tcp != nil {
			tcp := tcp.(*layers.TCP)
			if h.config.Checksum {
				err := tcp.SetNetworkLayerForChecksum(packet.NetworkLayer())
				if err != nil {
					log.Fatalf("Failed to set network layer for checksum: %s\n", err)
				}
			}
			c := Context{
				CaptureInfo: packet.Metadata().CaptureInfo,
			}
			stats.totalsz += len(tcp.Payload)
			h.assembler.AssembleWithContext(packet.NetworkLayer().NetworkFlow(), tcp, &c)
		}
		if count%h.config.Statsevery == 0 {
			ref := packet.Metadata().CaptureInfo.Timestamp
			flushed, closed := h.assembler.FlushWithOptions(reassembly.FlushOptions{T: ref.Add(-h.config.Timeout), TC: ref.Add(-h.config.CloseTimeout)})
			Debug("Forced flush: %d flushed, %d closed (%s)", flushed, closed, ref)
		}

		done := h.config.Maxcount > 0 && count >= h.config.Maxcount
		if count%h.config.Statsevery == 0 || done {
			errorsMapMutex.Lock()
			errorMapLen := len(errorsMap)
			errorsMapMutex.Unlock()
			Debug("Processed %v packets (%v bytes) in %v (errors: %v, errTypes:%v)\n", count, bytes, time.Since(start), errors, errorMapLen)
		}
	}
}

func (h *tcpAssembler) Stop() {
	closed := h.assembler.FlushAll()
	// Debug("Final flush: %d closed", closed)
	log.Printf("Final flush: %d closed", closed)
	if logLevel >= 2 {
		h.streamPool.Dump()
	}

	h.streamFactory.WaitGoRoutines()
	// Debug("%s\n", h.assembler.Dump())
	log.Printf("%s\n", h.assembler.Dump())
	if !h.config.Nodefrag {
		fmt.Printf("IPdefrag:\t\t%d\n", stats.ipdefrag)
	}
	fmt.Printf("TCP stats:\n")
	fmt.Printf(" missed bytes:\t\t%d\n", stats.missedBytes)
	fmt.Printf(" total packets:\t\t%d\n", stats.pkt)
	fmt.Printf(" rejected FSM:\t\t%d\n", stats.rejectFsm)
	fmt.Printf(" rejected Options:\t%d\n", stats.rejectOpt)
	fmt.Printf(" reassembled bytes:\t%d\n", stats.sz)
	fmt.Printf(" total TCP bytes:\t%d\n", stats.totalsz)
	fmt.Printf(" conn rejected FSM:\t%d\n", stats.rejectConnFsm)
	fmt.Printf(" reassembled chunks:\t%d\n", stats.reassembled)
	fmt.Printf(" out-of-order packets:\t%d\n", stats.outOfOrderPackets)
	fmt.Printf(" out-of-order bytes:\t%d\n", stats.outOfOrderBytes)
	fmt.Printf(" biggest-chunk packets:\t%d\n", stats.biggestChunkPackets)
	fmt.Printf(" biggest-chunk bytes:\t%d\n", stats.biggestChunkBytes)
	fmt.Printf(" overlap packets:\t%d\n", stats.overlapPackets)
	fmt.Printf(" overlap bytes:\t\t%d\n", stats.overlapBytes)
	fmt.Printf("Errors: %d\n", errors)
	for e, _ := range errorsMap {
		fmt.Printf(" %s:\t\t%d\n", e, errorsMap[e])
	}
}

func handleHttpEvents(events chan httpEvent) {
	for {
		select {
		case event := <-events:

			ev := libhoney.NewEvent()
			ev.AddField("duration_ms", event.duration.Microseconds())
			ev.AddField("http.source_ip", event.srcIp)
			ev.AddField("http.destination_ip", event.dstIp)
			if event.request != nil {
				ev.AddField("name", fmt.Sprintf("HTTP %s", event.request.Method))
				ev.AddField(string(semconv.HTTPMethodKey), event.request.Method)
				ev.AddField(string(semconv.HTTPURLKey), event.request.RequestURI)
				ev.AddField("http.request.body", fmt.Sprintf("%v", event.request.Body))
				ev.AddField("http.request.headers", fmt.Sprintf("%v", event.request.Header))
			} else {
				ev.AddField("name", "HTTP")
				ev.AddField("http.request.missing", "no request on this event")
			}

			if event.response != nil {
				ev.AddField(string(semconv.HTTPStatusCodeKey), event.response.StatusCode)
				ev.AddField("http.response.body", event.response.Body)
				ev.AddField("http.response.headers", event.response.Header)
			} else {
				ev.AddField("http.response.missing", "no request on this event")
			}

			//TODO: Body size produces a runtime error, commenting out for now.
			// requestSize := getBodySize(event.request.Body)
			// ev.AddField("http.request.body.size", requestSize)
			// responseSize := getBodySize(event.response.Body)
			// ev.AddField("http.response.body.size", responseSize)

			err := ev.Send()
			if err != nil {
				log.Printf("error sending event: %v\n", err)
			}
		}
	}
}

func getBodySize(r io.ReadCloser) int {
	length := 0
	b, err := io.ReadAll(r)
	if err == nil {
		length = len(b)
		r.Close()
	}

	return length
}
