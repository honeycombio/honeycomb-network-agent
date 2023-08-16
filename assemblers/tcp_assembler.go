package assemblers

import (
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/ip4defrag"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"
	"github.com/google/gopacket/reassembly"
	"github.com/honeycombio/libhoney-go"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
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
	if len(flag.Args()) > 0 {
		bpffilter := strings.Join(flag.Args(), " ")
		log.Info().
			Str("bpf_filter", bpffilter).
			Msg("Using BPF filter")
		if err = handle.SetBPFFilter(bpffilter); err != nil {
			log.Fatal().
				Err(err).
				Str("bpf_filter", bpffilter).
				Msg("BPF filter error")
		}
	}

	packetSource := gopacket.NewPacketSource(handle, handle.LinkType())
	packetSource.Lazy = *lazy
	packetSource.NoCopy = true
	log.Info().Msg("Starting to read packets")

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
		if !h.config.nodefrag {
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
					log.Fatal().Err(err).Msg("Failed to set network layer for checksum")
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
			log.Debug().
				Int("flushed", flushed).
				Int("closed", closed).
				Time("packet_timestamp", ref).
				Msg("Forced flush")
		}

		done := h.config.maxcount > 0 && count >= h.config.maxcount
		if count%h.config.statsevery == 0 || done {
			log.Info().
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
	log.Printf("%s\n", h.assembler.Dump())
	log.Debug().
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
