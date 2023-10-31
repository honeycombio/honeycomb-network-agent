package assemblers

import (
	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/reassembly"
	"github.com/rs/zerolog/log"

	"github.com/honeycombio/honeycomb-network-agent/config"
)

type tcpStreamFactory struct {
	config     config.Config
	eventsChan chan Event
}

func NewTcpStreamFactory(config config.Config, eventsChan chan Event) tcpStreamFactory {
	return tcpStreamFactory{
		config:     config,
		eventsChan: eventsChan,
	}
}

func (factory *tcpStreamFactory) New(net, transport gopacket.Flow, tcp *layers.TCP, ac reassembly.AssemblerContext) reassembly.Stream {
	log.Debug().
		Str("net", net.String()).
		Str("transport", transport.String()).
		Msg("NEW tcp stream")
	IncrementActiveStreamCount()
	return NewTcpStream(net, transport, factory.config, factory.eventsChan)
}
