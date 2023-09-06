package assemblers

import (
	"fmt"
	"os"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/afpacket"
	"github.com/google/gopacket/ip4defrag"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"
	"github.com/google/gopacket/reassembly"
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
	packetSource  *gopacket.PacketSource
	streamFactory *tcpStreamFactory
	streamPool    *reassembly.StreamPool
	assembler     *reassembly.Assembler
	httpEvents    chan HttpEvent
}

func NewTcpAssembler(config config, httpEvents chan HttpEvent) tcpAssembler {
	var packetSource *gopacket.PacketSource
	var err error

	switch config.packetSource {
	case "pcap":
		packetSource, err = newPcapPacketSource(config)
		if err != nil {
			log.Fatal().Err(err).Msg("Failed to setup pcap handle")
		}
	case "afpacket":
		packetSource, err = newAfpacketSource(config)
		if err != nil {
			log.Fatal().Err(err).Msg("Failed to setup afpacket handle")
		}
	// TODO: other data sources (eg afpacket, pfring, etc)
	default:
		log.Fatal().Str("packet_source", config.packetSource).Msg("Unknown packet source")
	}

	packetSource.Lazy = config.Lazy
	packetSource.NoCopy = true

	streamFactory := NewTcpStreamFactory(config, httpEvents)
	streamPool := reassembly.NewStreamPool(&streamFactory)
	assembler := reassembly.NewAssembler(streamPool)

	return tcpAssembler{
		config:        &config,
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
	if zerolog.GlobalLevel() >= zerolog.DebugLevel {
		// this uses stdlib's log, but oh well
		h.streamPool.Dump()
	}

	h.streamFactory.WaitGoRoutines()
	log.Debug().
		Int("closed", closed).
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
		Msg("Stopping TCP assembler")
}

func newPcapPacketSource(config config) (*gopacket.PacketSource, error) {
	log.Info().
		Str("interface", config.Interface).
		Int("snaplen", config.Snaplen).
		Bool("promiscuous", config.Promiscuous).
		Str("bpf_filter", config.bpfFilter).
		Msg("Configuring pcap packet source")
	handle, err := pcap.OpenLive(config.Interface, int32(config.Snaplen), config.Promiscuous, time.Second)
	if err != nil {
		log.Fatal().
			Err(err).
			Msg("Failed to open a pcap handle")
		return nil, err
	}
	if config.bpfFilter != "" {
		if err = handle.SetBPFFilter(config.bpfFilter); err != nil {
			log.Fatal().
				Err(err).
				Msg("Error setting BPF filter")
			return nil, err
		}
	}

	go logPcapHandleStats(handle)
	return gopacket.NewPacketSource(
		handle,
		handle.LinkType(),
	), nil
}

func logPcapHandleStats(handle *pcap.Handle) {
	ticker := time.NewTicker(time.Second * 10)
	for {
		<-ticker.C
		stats, err := handle.Stats()
		if err != nil {
			log.Error().Err(err).Msg("Failed to get pcap handle stats")
			continue
		}
		log.Info().
			Int("packets_received", stats.PacketsReceived).
			Int("packets_dropped", stats.PacketsDropped).
			Int("packets_if_dropped", stats.PacketsIfDropped).
			Msg("Pcap handle stats")
	}
}

type afpacketHandle struct {
	TPacket *afpacket.TPacket
}

func (h afpacketHandle) ReadPacketData() ([]byte, gopacket.CaptureInfo, error) {
	data, ci, err := h.TPacket.ZeroCopyReadPacketData()
	if err != nil {
		return nil, gopacket.CaptureInfo{}, err
	}
	return data, gopacket.CaptureInfo{
		Timestamp:     ci.Timestamp,
		CaptureLength: ci.CaptureLength,
		Length:        ci.Length,
	}, nil
}

func newAfpacketSource(config config) (*gopacket.PacketSource, error) {
	log.Info().
		Str("interface", config.Interface).
		Int("snaplen", config.Snaplen).
		Bool("promiscuous", config.Promiscuous).
		Str("bpf_filter", config.bpfFilter).
		Msg("Configuring afpacket packet source")

	// handle, err := afpacket.NewTPacket()
	// handle, err := afpacket.NewTPacket(
	// 	afpacket.OptInterface(config.Interface),
	// 	afpacket.OptFrameSize(config.Snaplen),
	// 	afpacket.OptBlockSize(config.Snaplen*1024),
	// 	afpacket.OptNumBlocks(1),
	// 	afpacket.OptPollTimeout(time.Millisecond*100),
	// 	// afpacket.OptPollBlockSize(config.Snaplen*1024),
	// 	afpacket.OptPollNumBlocks(1),
	// 	afpacket.OptFanout(afpacket.FanoutHashWithDefrag),
	// )
	szFrame, szBlock, numBlocks, err := afpacketComputeSize(8, 65535, os.Getpagesize())
	if err != nil {
		log.Fatal().
			Err(err).
			Msg("Failed to compute afpacket buffer size")
		return nil, err
	}
	log.Info().
		Int("frame_size", szFrame).
		Int("block_size", szBlock).
		Int("num_blocks", numBlocks).
		Int("target_size_mb", 8).
		Int("snaplen", config.Snaplen).
		Int("page_size", os.Getpagesize()).
		Msg("afpacket buffer size")
	afpacketHandle, err := afpacket.NewTPacket(
		afpacket.OptFrameSize(szFrame),
		afpacket.OptBlockSize(szBlock),
		afpacket.OptNumBlocks(numBlocks),
		afpacket.OptAddVLANHeader(false),
		afpacket.OptPollTimeout(timeout),
		afpacket.SocketRaw,
		afpacket.TPacketVersion3)

	if err != nil {
		log.Fatal().
			Err(err).
			Msg("Failed to open a afpacket handle")
		return nil, err
	}

	go logAfpacketHandleStats(afpacketHandle)
	return gopacket.NewPacketSource(
		afpacketHandle,
		layers.LayerTypeEthernet,
	), nil
}

func logAfpacketHandleStats(handle *afpacket.TPacket) {
	ticker := time.NewTicker(time.Second * 10)
	for {
		<-ticker.C
		stats, err := handle.Stats()
		if err != nil {
			log.Error().Err(err).Msg("Failed to get afpacket handle stats")
			continue
		}
		log.Info().
			Int64("pools", stats.Polls).
			Int64("packets", stats.Packets).
			Msg("Afpacket handle stats")
	}
}

func afpacketComputeSize(targetSizeMb int, snaplen int, pageSize int) (
	frameSize int, blockSize int, numBlocks int, err error) {

	if snaplen < pageSize {
		frameSize = pageSize / (pageSize / snaplen)
	} else {
		frameSize = (snaplen/pageSize + 1) * pageSize
	}

	// 128 is the default from the gopacket library so just use that
	blockSize = frameSize * 128
	numBlocks = (targetSizeMb * 1024 * 1024) / blockSize

	if numBlocks == 0 {
		return 0, 0, 0, fmt.Errorf("Interface buffersize is too small")
	}

	return frameSize, blockSize, numBlocks, nil
}
