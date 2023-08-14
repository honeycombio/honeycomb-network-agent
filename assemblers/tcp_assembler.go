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
	config *config
	handle *pcap.Handle
	packetSource *gopacket.PacketSource
	streamFactory *tcpStreamFactory
	streamPool *reassembly.StreamPool
	assembler *reassembly.Assembler
}

func NewTcpAssembler(config config) tcpAssembler {
	var handle *pcap.Handle
	var err error

	// Set logging level
	if config.debug {
		logLevel = 2
	} else if config.verbose {
		logLevel = 1
	} else if config.quiet {
		logLevel = -1
	}
	errorsMap = make(map[string]uint)
	// Set up pcap packet capture
	if config.fname != "" {
		log.Printf("Reading from pcap dump %q", config.fname)
		handle, err = pcap.OpenOffline(config.fname)
	} else {
		log.Printf("Starting capture on interface %q", config.iface)
		handle, err = pcap.OpenLive(config.iface, int32(config.snaplen), true, pcap.BlockForever)
	}
	if err != nil {
		log.Fatal(err)
	}
	if len(flag.Args()) > 0 {
		bpffilter := strings.Join(flag.Args(), " ")
		// Info("Using BPF filter %q\n", bpffilter)
		log.Printf("Using BPF filter %q\n", bpffilter)
		if err = handle.SetBPFFilter(bpffilter); err != nil {
			log.Fatal("BPF filter error:", err)
		}
	}

	packetSource := gopacket.NewPacketSource(handle, handle.LinkType())
	packetSource.Lazy = config.lazy
	packetSource.NoCopy = true
	// Info("Starting to read packets\n")
	log.Printf("Starting to read packets\n")

	streamFactory := &tcpStreamFactory{}
	streamPool := reassembly.NewStreamPool(streamFactory)
	assembler := reassembly.NewAssembler(streamPool)

	return tcpAssembler{
		handle: handle,
		packetSource: packetSource,
		streamFactory: streamFactory,
		streamPool: streamPool,
		assembler: assembler,
	}
}

func (h *tcpAssembler) Start() {
	count := 0
	bytes := int64(0)
	start := time.Now()
	defragger := ip4defrag.NewIPv4Defragmenter()

	for packet := range h.packetSource.Packets() {
		count++
		// Debug("PACKET #%d\n", count)
		log.Printf("PACKET #%d\n", count)
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
