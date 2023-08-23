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
	log.Info().Msg("Starting TCP assembler")
	flushTicker := time.NewTicker(time.Second * 5)
	count := 0
	bytes := int64(0)
	start := time.Now()
	defragger := ip4defrag.NewIPv4Defragmenter()

	for {
		select {
		case <-flushTicker.C:
			flushed, closed := h.assembler.FlushCloseOlderThan(time.Now().Add(-h.config.Timeout))
			log.Debug().
				Int("flushed", flushed).
				Int("closed", closed).
				Msg("Flushing old streams")
		case packet := <-h.packetSource.Packets():
			count++
			data := packet.Data()
			bytes += int64(len(data))
			// defrag the IPv4 packet if required
			if ipv4Layer := packet.Layer(layers.LayerTypeIPv4); ipv4Layer != nil {
				ipv4 := ipv4Layer.(*layers.IPv4)
				newipv4, err := defragger.DefragIPv4(ipv4)
				if err != nil {
					log.Debug().Err(err).Msg("Error while de-fragmenting")
					continue
				}
				if newipv4 == nil {
					log.Debug().Msg("Ingoring packet fragment")
					continue
				}

				// decode the packet if required
				if newipv4.Length != ipv4.Length {
					stats.ipdefrag++
					builder, ok := packet.(gopacket.PacketBuilder)
					if !ok {
						log.Debug().Msg("Unable to decode packet - does not contain PacketBuilder")
					}
					nextDecoder := newipv4.NextLayerType()
					nextDecoder.Decode(newipv4.Payload, builder)
				}
			}

			// process TCP packet
			if tcpLayer := packet.Layer(layers.LayerTypeTCP); tcpLayer != nil {
				tcp := tcpLayer.(*layers.TCP)
				if h.config.Checksum {
					err := tcp.SetNetworkLayerForChecksum(packet.NetworkLayer())
					if err != nil {
						log.Debug().Err(err).Msg("Failed to set network layer for checksum")
						continue
					}
				}
				context := Context{
					CaptureInfo: packet.Metadata().CaptureInfo,
				}
				stats.totalsz += len(tcp.Payload)
				h.assembler.AssembleWithContext(packet.NetworkLayer().NetworkFlow(), tcp, &context)
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
