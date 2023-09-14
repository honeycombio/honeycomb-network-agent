package assemblers

import (
	"sync"
	"sync/atomic"

	"github.com/honeycombio/ebpf-agent/config"
	"github.com/honeycombio/gopacket"
	"github.com/honeycombio/gopacket/layers"
	"github.com/honeycombio/gopacket/reassembly"
	"github.com/rs/zerolog/log"
)

var streamId uint64 = 0

type tcpStreamFactory struct {
	config     config.Config
	wg         sync.WaitGroup
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
	streamId := atomic.AddUint64(&streamId, 1)
	stream := NewTcpStream(streamId, net, transport, factory.config, factory.httpEvents)

	factory.wg.Add(2)
	go stream.client.run(&factory.wg)
	go stream.server.run(&factory.wg)
	return stream
}

func (factory *tcpStreamFactory) WaitGoRoutines() {
	factory.wg.Wait()
}
