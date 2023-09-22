package assemblers

import (
	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/reassembly"
	"github.com/honeycombio/honeycomb-network-agent/config"
	"github.com/rs/zerolog/log"
)

type tcpStreamFactory struct {
	config     config.Config
	httpEvents chan HttpEvent
}

func NewTcpStreamFactory(config config.Config, httpEvents chan HttpEvent) tcpStreamFactory {
	return tcpStreamFactory{
		config:     config,
		httpEvents: httpEvents,
	}
}

func (factory *tcpStreamFactory) New(net, transport gopacket.Flow, tcp *layers.TCP, ac reassembly.AssemblerContext) reassembly.Stream {
	log.Debug().
		Str("net", net.String()).
		Str("transport", transport.String()).
		Msg("NEW tcp stream")
	streamId := IncrementStreamCount()
	stream := NewTcpStream(streamId, net, transport, factory.config, factory.httpEvents)

	// increment the number of active streams
	IncrementActiveStreamCount()
	return stream
}
