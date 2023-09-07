package assemblers

import (
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/pcap"
	"github.com/honeycombio/libhoney-go"
	"github.com/rs/zerolog/log"
)

func newPcapPacketSource(config config) (*gopacket.PacketSource, error) {
	log.Info().
		Str("interface", config.Interface).
		Int("snaplen", config.Snaplen).
		Bool("promiscuous", config.Promiscuous).
		Str("bpf_filter", config.bpfFilter).
		Msg("Configuring pcap packet source")
	handle, err := pcap.OpenLive(config.Interface, int32(config.Snaplen), config.Promiscuous, time.Second)
	if err != nil {
		return nil, err
	}
	if config.bpfFilter != "" {
		if err = handle.SetBPFFilter(config.bpfFilter); err != nil {
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
		stats, err := handle.Stats()
		if err != nil {
			log.Error().Err(err).Msg("Failed to get pcap handle stats")
			continue
		}
		// TODO use config for different dataset for stats telemetry
		// create libhoney event
		ev := libhoney.NewEvent()
		ev.Dataset = "hny-ebpf-agent-stats"
		ev.AddField("name", "tcp_assembler_pcap")
		ev.AddField("pcap.packets_received", stats.PacketsReceived)
		ev.AddField("pcap.packets_dropped", stats.PacketsDropped)
		ev.AddField("pcap.packets_if_dropped", stats.PacketsIfDropped)
		log.Info().
			Int("pcap.packets_received", stats.PacketsReceived).
			Int("pcap.packets_dropped", stats.PacketsDropped).
			Int("pcap.packets_if_dropped", stats.PacketsIfDropped).
			Msg("Pcap handle stats")
		ev.Send()
	}
}
