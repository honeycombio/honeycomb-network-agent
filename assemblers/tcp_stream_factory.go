package assemblers

import (
	"sync"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/reassembly"
	"github.com/honeycombio/honeycomb-network-agent/config"
	"github.com/rs/zerolog/log"
)

type tcpStreamFactory struct {
	config     config.Config
	wg         sync.WaitGroup
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

func (factory *tcpStreamFactory) WaitGoRoutines() {
	factory.wg.Wait()
}
