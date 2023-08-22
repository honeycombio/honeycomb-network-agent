package assemblers

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/ip4defrag"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"
	"github.com/google/gopacket/reassembly"
	"github.com/honeycombio/ebpf-agent/utils"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
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
	httpEvents    chan HttpEvent
}

func NewTcpAssembler(config config, httpEvents chan HttpEvent, k8sClient *utils.CachedK8sClient) tcpAssembler {
	var handle *pcap.Handle
	var err error

	// Set up pcap packet capture
	if *fname != "" {
		log.Info().
			Str("filename", *fname).
			Msg("Reading from pcap dump")
		handle, err = pcap.OpenOffline(*fname)
	} else {
		log.Info().
			Str("interface", *iface).
			Msg("Starting capture")
		handle, err = pcap.OpenLive(*iface, int32(*snaplen), true, pcap.BlockForever)
	}
	if err != nil {
		log.Fatal().
			Err(err).
			Msg("Failed to open a pcap handle")
	}
	// if len(flag.Args()) > 0 {
	// bpffilter := strings.Join(flag.Args(), " ")
	bpffilter := "tcp"
	pods := k8sClient.GetPodsByNamespace("greetings")
	hosts := []string{}
	for _, pod := range pods {
		hosts = append(hosts, pod.Status.PodIP)
	}
	bpffilter += fmt.Sprintf(" and (host %s)", strings.Join(hosts, " or "))
	log.Info().
		Str("bpf_filter", bpffilter).
		Msg("Using BPF filter")
	if err = handle.SetBPFFilter(bpffilter); err != nil {
		log.Fatal().
			Err(err).
			Str("bpf_filter", bpffilter).
			Msg("BPF filter error")
	}
	// }

	packetSource := gopacket.NewPacketSource(handle, handle.LinkType())
	packetSource.Lazy = *lazy
	packetSource.NoCopy = true
	log.Info().Msg("Starting to read packets")

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
				log.Fatal().Err(err).Msg("Error while de-fragmenting")
			} else if newip4 == nil {
				log.Debug().Msg("Fragment...\n")
				continue // packet fragment, we don't have whole packet yet.
			}
			if newip4.Length != l {
				stats.ipdefrag++
				log.Debug().
					Str("network_layer_type", newip4.NextLayerType().String()).
					Msg("Decoding re-assembled packet")
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
					log.Fatal().Err(err).Msg("Failed to set network layer for checksum")
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
			flushed, closed := h.assembler.FlushCloseOlderThan(time.Now().Add(-h.config.Timeout))
			log.Debug().
				Int("flushed", flushed).
				Int("closed", closed).
				Time("packet_timestamp", ref).
				Msg("Forced flush")
		}

		done := h.config.Maxcount > 0 && count >= h.config.Maxcount
		if count%h.config.Statsevery == 0 || done {
			log.Debug().
				Int("processed_count_since_start", count).
				Int64("milliseconds_since_start", time.Since(start).Milliseconds()).
				Int64("bytes", bytes).
				Msg("Processed Packets")
		}
	}
}

func (h *tcpAssembler) Stop() {
	closed := h.assembler.FlushAll()
	// Debug("Final flush: %d closed", closed)
	log.Debug().
		Int("closed", closed).
		Msg("Final flush")
	if zerolog.GlobalLevel() >= zerolog.DebugLevel {
		// this uses stdlib's log, but oh well
		h.streamPool.Dump()
	}

	h.streamFactory.WaitGoRoutines()
	log.Debug().
		Str("assember_page_usage", h.assembler.Dump()).
		Int("IPdefrag", stats.ipdefrag).
		Int("missed_bytes", stats.missedBytes).
		Int("total_packets", stats.pkt).
		Int("rejected_FSM", stats.rejectFsm).
		Int("rejected_Options", stats.rejectOpt).
		Int("reassembled_bytes", stats.sz).
		Int("total_TCP_bytes", stats.totalsz).
		Int("conn_rejected_FSM", stats.rejectConnFsm).
		Int("reassembled_chunks", stats.reassembled).
		Int("out_of_order_packets", stats.outOfOrderPackets).
		Int("out_of_order_bytes", stats.outOfOrderBytes).
		Int("biggest_chunk_packets", stats.biggestChunkPackets).
		Int("biggest_chunk_bytes", stats.biggestChunkBytes).
		Int("overlap_packets", stats.overlapPackets).
		Int("overlap_bytes", stats.overlapBytes).
		Msg("Stop")
}
