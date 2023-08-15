package assemblers

import (
	"fmt"
	"sync"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/reassembly"
)

type tcpStreamFactory struct {
	wg sync.WaitGroup
}

func (factory *tcpStreamFactory) New(net, transport gopacket.Flow, tcp *layers.TCP, ac reassembly.AssemblerContext) reassembly.Stream {
	Debug("* NEW: %s %s\n", net, transport)
	fsmOptions := reassembly.TCPSimpleFSMOptions{
		SupportMissingEstablishment: true,
	}
	stream := &tcpStream{
		net:        net,
		transport:  transport,
		tcpstate:   reassembly.NewTCPSimpleFSM(fsmOptions),
		ident:      fmt.Sprintf("%s:%s", net, transport),
		optchecker: reassembly.NewTCPOptionCheck(),
	}

	stream.client = httpReader{
		bytes:    make(chan []byte),
		ident:    fmt.Sprintf("%s %s", net, transport),
		parent:   stream,
		isClient: true,
		srcIp:    fmt.Sprintf("%s", net.Src()),
		dstIp:    fmt.Sprintf("%s", net.Dst()),
		srcPort:  fmt.Sprintf("%s", transport.Src()),
		dstPort:  fmt.Sprintf("%s", transport.Dst()),
	}
	stream.server = httpReader{
		bytes:   make(chan []byte),
		ident:   fmt.Sprintf("%s %s", net.Reverse(), transport.Reverse()),
		parent:  stream,
		srcIp:   fmt.Sprintf("%s", net.Reverse().Src()),
		dstIp:   fmt.Sprintf("%s", net.Reverse().Dst()),
		srcPort: fmt.Sprintf("%s", transport.Reverse().Src()),
		dstPort: fmt.Sprintf("%s", transport.Reverse().Dst()),
	}
	factory.wg.Add(2)
	go stream.client.run(&factory.wg)
	go stream.server.run(&factory.wg)
	return stream
}

func (factory *tcpStreamFactory) WaitGoRoutines() {
	factory.wg.Wait()
}
