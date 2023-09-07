package assemblers

import (
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/reassembly"
	"github.com/honeycombio/ebpf-agent/config"
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
	fsmOptions := reassembly.TCPSimpleFSMOptions{
		SupportMissingEstablishment: true,
	}
	streamId := atomic.AddUint64(&streamId, 1)
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
		bytes:    make(chan []byte),
		parent:   stream,
		isClient: true,
		srcIp:    fmt.Sprintf("%s", net.Src()),
		dstIp:    fmt.Sprintf("%s", net.Dst()),
		srcPort:  fmt.Sprintf("%s", transport.Src()),
		dstPort:  fmt.Sprintf("%s", transport.Dst()),
		messages: make(chan message, factory.config.ChannelBufferSize),
	}
	stream.server = httpReader{
		bytes:    make(chan []byte),
		parent:   stream,
		isClient: false,
		srcIp:    fmt.Sprintf("%s", net.Reverse().Src()),
		dstIp:    fmt.Sprintf("%s", net.Reverse().Dst()),
		srcPort:  fmt.Sprintf("%s", transport.Reverse().Src()),
		dstPort:  fmt.Sprintf("%s", transport.Reverse().Dst()),
		messages: make(chan message, factory.config.ChannelBufferSize),
	}
	factory.wg.Add(2)
	go stream.client.run(&factory.wg)
	go stream.server.run(&factory.wg)
	return stream
}

func (factory *tcpStreamFactory) WaitGoRoutines() {
	factory.wg.Wait()
}
