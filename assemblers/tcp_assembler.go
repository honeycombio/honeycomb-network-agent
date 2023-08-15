package assemblers

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/ip4defrag"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"
	"github.com/google/gopacket/reassembly"
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
	go handleHttpEvents(h.httpEvents)

	count := 0
	bytes := int64(0)
	start := time.Now()
	defragger := ip4defrag.NewIPv4Defragmenter()
	for packet := range h.packetSource.Packets() {
		count++
		// Debug("PACKET #%d\n", count)
		// log.Printf("PACKET #%d\n", count)
		data := packet.Data()
		bytes += int64(len(data))
		// defrag the IPv4 packet if required
		if !h.config.nodefrag {
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
			if h.config.checksum {
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
		if count%h.config.statsevery == 0 {
			ref := packet.Metadata().CaptureInfo.Timestamp
			flushed, closed := h.assembler.FlushWithOptions(reassembly.FlushOptions{T: ref.Add(-h.config.timeout), TC: ref.Add(-h.config.closeTimeout)})
			// Debug("Forced flush: %d flushed, %d closed (%s)", flushed, closed, ref)
			log.Printf("Forced flush: %d flushed, %d closed (%s)", flushed, closed, ref)
		}

		done := h.config.maxcount > 0 && count >= h.config.maxcount
		if count%h.config.statsevery == 0 || done {
			errorsMapMutex.Lock()
			errorMapLen := len(errorsMap)
			errorsMapMutex.Unlock()
			fmt.Fprintf(os.Stderr, "Processed %v packets (%v bytes) in %v (errors: %v, errTypes:%v)\n", count, bytes, time.Since(start), errors, errorMapLen)
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
	if !h.config.nodefrag {
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
			// fmt.Printf("%s\n\n", event)

			// ignore health and ready checks for now
			if strings.HasPrefix(event.request.RequestURI, "/health") || strings.HasPrefix(event.request.RequestURI, "/ready") {
				continue
			}

			log.Printf("request complete: %s %s - %d", event.request.Method, event.request.RequestURI, event.duration.Microseconds())

			// eventAttrs := map[string]string{
			// 	"name":                     fmt.Sprintf("HTTP %s", req.Method),
			// 	"http.request_method":      req.Method,
			// 	"http.request_ident":       h.ident,
			// 	"http.request_source_ip":   h.srcIp,
			// 	"http.request_source_port": h.srcPort,
			// 	"http.request_dest_ip":     h.dstIp,
			// 	"http.request_dest_port":   h.dstPort,
			// 	"http.request_url":         req.RequestURI,
			// 	"http.request_body":        fmt.Sprintf("%v", req.Body),
			// 	"http.request_headers":     fmt.Sprintf("%v", req.Header),
			// 	"http.h_request_bytes":     string(<-h.bytes),
			// }

			// ev := libhoney.NewEvent()
			// ev.Add(eventAttrs)
			// ev.AddField("http.response_ident", h.ident)
			// ev.AddField("http.response_body", res.Body)
			// ev.AddField("http.response_code", res.StatusCode)
			// ev.AddField("http.response_headers", res.Header)
			// ev.AddField("http.h_response_bytes", h.bytes)
			// ev.AddField("http.response_info", responseInfo)
			// ev.AddField("http.response_mutex", h.srcIp)
			// ev.AddField("http.response_source_ip", h.srcIp)
			// ev.AddField("http.response_source_port", h.srcPort)
			// ev.AddField("http.response_dest_ip", h.dstIp)
			// ev.AddField("http.response_dest_port", h.dstPort)

			// Do we care about response decoding right now?
			// encoding := res.Header["Content-Encoding"]
			// if err == nil {
			// 	base := url.QueryEscape(path.Base(eventAttrs["http.request_url"]))
			// 	if err != nil {
			// 		base = "incomplete-" + base
			// 	}
			// 	if len(base) > 250 {
			// 		base = base[:250] + "..."
			// 	}
			// 	target := base
			// 	n := 0
			// 	for true {
			// 		_, err := os.Stat(target)
			// 		//if os.IsNotExist(err) != nil {
			// 		if err != nil {
			// 			break
			// 		}
			// 		target = fmt.Sprintf("%s-%d", base, n)
			// 		n++
			// 	}
			// 	f, err := os.Create(target)
			// 	if err != nil {
			// 		Error("HTTP-create", "Cannot create %s: %s\n", target, err)
			// 		continue
			// 	}
			// 	var r io.Reader
			// 	r = bytes.NewBuffer(body)
			// 	if len(encoding) > 0 && (encoding[0] == "gzip" || encoding[0] == "deflate") {
			// 		r, err = gzip.NewReader(r)
			// 		if err != nil {
			// 			Error("HTTP-gunzip", "Failed to gzip decode: %s", err)
			// 		}
			// 	}
			// 	if err == nil {
			// 		w, err := io.Copy(f, r)
			// 		if _, ok := r.(*gzip.Reader); ok {
			// 			r.(*gzip.Reader).Close()
			// 		}
			// 		f.Close()
			// 		if err != nil {
			// 			Error("HTTP-save", "%s: failed to save %s (l:%d): %s\n", h.ident, target, w, err)
			// 		} else {
			// 			Info("%s: Saved %s (l:%d)\n", h.ident, target, w)
			// 		}
			// 	}
			// }
		}
	}
}
