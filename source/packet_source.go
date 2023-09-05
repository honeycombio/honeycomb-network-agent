package source

import (
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/pcap"
	"github.com/honeycombio/ebpf-agent/config"
	"github.com/rs/zerolog/log"
)

func NewPacketSource(config config.Config) (*gopacket.PacketSource, error) {
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

	return packetSource, nil
}

func newPcapPacketSource(config config.Config) (*gopacket.PacketSource, error) {
	log.Info().
		Str("interface", config.Interface).
		Int("snaplen", config.Snaplen).
		Bool("promiscuous", config.Promiscuous).
		Str("bpf_filter", config.BpfFilter).
		Msg("Configuring pcap packet source")
	handle, err := pcap.OpenLive(config.Interface, int32(config.Snaplen), config.Promiscuous, time.Second)
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

	return gopacket.NewPacketSource(
		handle,
		handle.LinkType(),
	), nil
}
