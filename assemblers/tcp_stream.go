package assemblers

import (
	"fmt"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/reassembly"
	"github.com/honeycombio/honeycomb-network-agent/config"
	"github.com/rs/zerolog/log"
)

// tcpStream has two unidirectional tcpReaders, one for client and one for server
type tcpStream struct {
	id         uint64
	ident      string
	config     config.Config
	tcpstate   *reassembly.TCPSimpleFSM
	fsmerr     bool
	optchecker reassembly.TCPOptionCheck
	client     *tcpReader
	server     *tcpReader
}

func NewTcpStream(streamId uint64, net gopacket.Flow, transport gopacket.Flow, config config.Config, httpEvents chan HttpEvent) *tcpStream {
	ident := fmt.Sprintf("%s:%s:%d", net, transport, streamId)
	matcher := newRequestResponseMatcher()
	return &tcpStream{
		id:     streamId,
		ident:  ident,
		config: config,
		tcpstate: reassembly.NewTCPSimpleFSM(reassembly.TCPSimpleFSMOptions{
			SupportMissingEstablishment: true,
		}),
		fsmerr:     false, // TODO: verify whether we need this
		optchecker: reassembly.NewTCPOptionCheck(),
		client:     NewTcpReader(ident, true, net, transport, matcher, httpEvents),
		server:     NewTcpReader(ident, false, net.Reverse(), transport.Reverse(), matcher, httpEvents),
	}
}

func (stream *tcpStream) Accept(tcp *layers.TCP, ci gopacket.CaptureInfo, dir reassembly.TCPFlowDirection, nextSeq reassembly.Sequence, start *bool, ac reassembly.AssemblerContext) bool {
	// FSM
	if !stream.tcpstate.CheckState(tcp, dir) {
		// Error("FSM", "%s: Packet rejected by FSM (state:%s)\n", t.ident, t.tcpstate.String())
		stats.rejectFsm++
		if !stream.fsmerr {
			stream.fsmerr = true
			stats.rejectConnFsm++
		}
		if !stream.config.Ignorefsmerr {
			return false
		}
	}
	// Options
	err := stream.optchecker.Accept(tcp, ci, dir, nextSeq, start)
	if err != nil {
		// Error("OptionChecker", "%s: Packet rejected by OptionChecker: %s\n", t.ident, err)
		stats.rejectOpt++
		if !stream.config.Nooptcheck {
			return false
		}
	}
	// Checksum
	accept := true
	if stream.config.Checksum {
		c, err := tcp.ComputeChecksum()
		if err != nil {
			log.Error().
				Err(err).
				Str("stream_ident", stream.ident).
				Msg("ChecksumCompute")
			accept = false
		} else if c != 0x0 {
			log.Error().
				Str("stream_ident", stream.ident).
				Uint16("checksum", c).
				Msg("InvalidChecksum")
			accept = false
		}
	}
	if !accept {
		stats.rejectOpt++
	}
	return accept
}

func (stream *tcpStream) ReassembledSG(sg reassembly.ScatterGather, ac reassembly.AssemblerContext) {
	dir, _, _, _ := sg.Info()
	if dir == reassembly.TCPDirClientToServer {
		stream.client.reassembledSG(sg, ac)
	} else {
		stream.server.reassembledSG(sg, ac)
	}
}

// ReassemblyComplete is called when the TCP assembler believes a stream has completed.
func (stream *tcpStream) ReassemblyComplete(ac reassembly.AssemblerContext) bool {
	log.Debug().
		Uint64("stream_id", stream.id).
		Msg("Connection closed")

	// decrement the number of active streams
	DecrementActiveStreamCount()
	return true // remove the connection, heck with the last ACK
}
