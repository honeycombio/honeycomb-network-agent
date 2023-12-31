package assemblers

import (
	"context"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/ip4defrag"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcap"
	"github.com/gopacket/gopacket/reassembly"
	"github.com/honeycombio/libhoney-go"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/honeycombio/honeycomb-network-agent/config"
)

var stats struct {
	ipdefrag int
	totalsz  int

	// Below stats could be accessed concurrently, so explicitly
	// mark them as atomic.
	rejectFsm         atomic.Uint64
	rejectOpt         atomic.Uint64
	rejectConnFsm     atomic.Uint64
	total_streams     atomic.Uint64
	active_streams    atomic.Uint64
	source_received   atomic.Uint64
	source_dropped    atomic.Uint64
	source_if_dropped atomic.Uint64
}

func IncrementStreamCount() uint64 {
	return stats.total_streams.Add(1)
}

func IncrementActiveStreamCount() {
	stats.active_streams.Add(1)
}

func DecrementActiveStreamCount() {
	stats.active_streams.Add(^uint64(0))
}

type Context struct {
	CaptureInfo gopacket.CaptureInfo
	seq, ack    reassembly.Sequence
}

func (c *Context) GetCaptureInfo() gopacket.CaptureInfo {
	return c.CaptureInfo
}

type tcpAssembler struct {
	startedAt     time.Time
	config        config.Config
	packetSource  *gopacket.PacketSource
	streamFactory *tcpStreamFactory
	streamPool    *reassembly.StreamPool
	assembler     *reassembly.Assembler
	eventsChan    chan Event
}

func NewTcpAssembler(config config.Config, eventsChan chan Event) tcpAssembler {
	var packetSource *gopacket.PacketSource
	var err error

	switch config.PacketSource {
	case "pcap":
		packetSource, err = newPcapPacketSource(config)
		if err != nil {
			log.Fatal().Err(err).Msg("Failed to setup pcap handle")
		}
	// TODO: other data sources (eg afpacket, pfring, etc)
	default:
		log.Fatal().Str("packet_source", config.PacketSource).Msg("Unknown packet source")
	}

	packetSource.Lazy = config.Lazy
	packetSource.NoCopy = true

	streamFactory := NewTcpStreamFactory(config, eventsChan)
	streamPool := reassembly.NewStreamPool(&streamFactory)
	assembler := reassembly.NewAssembler(streamPool)

	// Set total max pages and per-connection max pages -- this is very important to limit memory usage
	assembler.AssemblerOptions.MaxBufferedPagesTotal = config.MaxBufferedPagesTotal
	assembler.AssemblerOptions.MaxBufferedPagesPerConnection = config.MaxBufferedPagesPerConnection

	return tcpAssembler{
		config:        config,
		packetSource:  packetSource,
		streamFactory: &streamFactory,
		streamPool:    streamPool,
		assembler:     assembler,
		eventsChan:    eventsChan,
	}
}

func (h *tcpAssembler) Start(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()

	// Tick on the tightest loop. The flush timeout is the shorter of the two timeouts using this ticker.
	// Tick even more frequently than the flush interval (4 is somewhat arbitrary)
	flushCloseTicker := time.NewTicker(h.config.StreamFlushTimeout / 4)
	statsTicker := time.NewTicker(time.Second * 10)
	h.startedAt = time.Now()
	defragger := ip4defrag.NewIPv4Defragmenter()

	for {
		select {
		case <-ctx.Done():
			h.Stop()
			return
		case <-flushCloseTicker.C:
			flushed, closed := h.assembler.FlushWithOptions(
				reassembly.FlushOptions{
					T:  time.Now().Add(-h.config.StreamFlushTimeout),
					TC: time.Now().Add(-h.config.StreamCloseTimeout),
				},
			)
			log.Debug().
				Int("flushed", flushed).
				Int("closed", closed).
				Msg("Flushing old streams")
		case <-statsTicker.C:
			h.logAssemblerStats()
		case packet := <-h.packetSource.Packets():
			if packet.NetworkLayer() == nil {
				// can't use this packet
				continue
			}

			// defrag the IPv4 packet if required
			if ipv4Layer := packet.Layer(layers.LayerTypeIPv4); ipv4Layer != nil {
				ipv4 := ipv4Layer.(*layers.IPv4)
				newipv4, err := defragger.DefragIPv4(ipv4)
				if err != nil {
					log.Debug().Err(err).Msg("Error while de-fragmenting")
					continue
				}
				if newipv4 == nil {
					log.Debug().Msg("Ignoring packet fragment")
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
					seq:         reassembly.Sequence(tcp.Seq),
					ack:         reassembly.Sequence(tcp.Ack),
				}
				stats.totalsz += len(tcp.Payload)
				h.assembler.AssembleWithContext(packet.NetworkLayer().NetworkFlow(), tcp, &context)
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

	h.logAssemblerStats()
	log.Debug().
		Int("closed", closed).
		Str("assembler_page_usage", h.assembler.Dump()).
		Msg("Stopping TCP assembler")
}

func (a *tcpAssembler) logAssemblerStats() {
	statsFields := map[string]interface{}{
		"uptime_ms":          time.Since(a.startedAt).Milliseconds(),
		"IPdefrag":           stats.ipdefrag,
		"rejected_FSM":       stats.rejectFsm.Load(),
		"rejected_Options":   stats.rejectOpt.Load(),
		"total_TCP_bytes":    stats.totalsz,
		"conn_rejected_FSM":  stats.rejectConnFsm.Load(),
		"source_received":    stats.source_received.Load(),
		"source_dropped":     stats.source_dropped.Load(),
		"source_if_dropped":  stats.source_if_dropped.Load(),
		"event_queue_length": len(a.eventsChan),
		"goroutines":         runtime.NumGoroutine(),
		"total_streams":      stats.total_streams.Load(),
		"active_streams":     stats.active_streams.Load(),
	}
	statsEvent := libhoney.NewEvent()
	statsEvent.Dataset = a.config.StatsDataset
	statsEvent.AddField("name", "tcp_assembler_stats")
	statsEvent.Add(statsFields)
	statsEvent.Send()

	log.Debug().
		Fields(statsFields).
		Msg("TCP assembler stats")
}

func newPcapPacketSource(config config.Config) (*gopacket.PacketSource, error) {
	log.Debug().
		Str("interface", config.Interface).
		Int("snaplen", config.Snaplen).
		Bool("promiscuous", config.Promiscuous).
		Str("bpf_filter", config.BpfFilter).
		Msg("Configuring pcap packet source")
	handle, err := pcap.OpenLive(config.Interface, int32(config.Snaplen), config.Promiscuous, pcap.BlockForever)
	if err != nil {
		log.Fatal().
			Err(err).
			Msg("Failed to open a pcap handle")
		return nil, err
	}
	if config.BpfFilter != "" {
		if err = handle.SetBPFFilter(config.BpfFilter); err != nil {
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
	// TODO make ticker configurable
	ticker := time.NewTicker(time.Second * 10)
	for {
		<-ticker.C
		pcapStats, err := handle.Stats()
		if err != nil {
			log.Error().Err(err).Msg("Failed to get pcap handle stats")
			continue
		}
		stats.source_received.Add(uint64(pcapStats.PacketsReceived))
		stats.source_dropped.Add(uint64(pcapStats.PacketsDropped))
		stats.source_if_dropped.Add(uint64(pcapStats.PacketsIfDropped))
	}
}
