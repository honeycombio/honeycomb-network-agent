package assemblers

import (
	"fmt"
	"sync"

	"github.com/honeycombio/gopacket"
	"github.com/honeycombio/gopacket/layers"
	"github.com/honeycombio/gopacket/reassembly"
	"github.com/honeycombio/honeycomb-network-agent/config"
	"github.com/rs/zerolog/log"
)

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
	fsmOptions := reassembly.TCPSimpleFSMOptions{
		SupportMissingEstablishment: true,
	}

	// increment total stream count and use as stream id
	streamId := IncrementStreamCount()
	stream := &tcpStream{
		config:     factory.config,
		id:         streamId,
		net:        net,
		transport:  transport,
		tcpstate:   reassembly.NewTCPSimpleFSM(fsmOptions),
		ident:      fmt.Sprintf("%s:%s:%d", net, transport, streamId),
		optchecker: reassembly.NewTCPOptionCheck(),
		matcher:    newRequestResponseMatcher(),
		events:     factory.httpEvents,
	}

	stream.client = httpReader{
		parent:   stream,
		isClient: true,
		srcIp:    net.Src().String(),
		dstIp:    net.Dst().String(),
		srcPort:  transport.Src().String(),
		dstPort:  transport.Dst().String(),
		messages: make(chan message, factory.config.ChannelBufferSize),
	}
	stream.server = httpReader{
		parent:   stream,
		isClient: false,
		srcIp:    net.Reverse().Src().String(),
		dstIp:    net.Reverse().Dst().String(),
		srcPort:  transport.Reverse().Src().String(),
		dstPort:  transport.Reverse().Dst().String(),
		messages: make(chan message, factory.config.ChannelBufferSize),
	}
	factory.wg.Add(2)
	go stream.client.run(&factory.wg)
	go stream.server.run(&factory.wg)

	// increment the number of active streams
	IncrementActiveStreamCount()
	return stream
}

func (factory *tcpStreamFactory) WaitGoRoutines() {
	factory.wg.Wait()
}
