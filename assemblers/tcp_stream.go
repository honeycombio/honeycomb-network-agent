package assemblers

import (
	"bufio"
	"bytes"
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
	httpEvents chan HttpEvent
	srcIP      string
	dstIP      string
	srcPort    string
	dstPort    string
	buffer     *bufio.Reader
	parsers    []Parser
}

func NewTcpStream(streamId uint64, net gopacket.Flow, transport gopacket.Flow, config config.Config, httpEvents chan HttpEvent) *tcpStream {
	return &tcpStream{
		id:     streamId,
		ident:  fmt.Sprintf("%s:%s:%d", net, transport, streamId),
		config: config,
		tcpstate: reassembly.NewTCPSimpleFSM(reassembly.TCPSimpleFSMOptions{
			SupportMissingEstablishment: true,
		}),
		fsmerr:     false, // TODO: verify whether we need this
		optchecker: reassembly.NewTCPOptionCheck(),
		httpEvents: httpEvents,
		srcIP:      net.Src().String(),
		dstIP:      net.Dst().String(),
		srcPort:    transport.Src().String(),
		dstPort:    transport.Dst().String(),
		buffer:     bufio.NewReader(bytes.NewReader(nil)),
		parsers: []Parser{
			NewHttpParser(),
		},
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
	// Get the direction of the packet (client to server or server to client)
	dir, _, _, _ := sg.Info()
	isClient := dir == reassembly.TCPDirClientToServer

	// get the data from the packet
	len, _ := sg.Lengths()
	data := sg.Fetch(len)

	// gwt our custom context that includes the TCP seq/ack numbers
	ctx, ok := ac.(*Context)
	if !ok {
		log.Warn().
			Msg("Failed to cast ScatterGather to ContextWithSeq")
	}

	// get the number of packets that made up this request
	packetCount := sg.Stats().Packets

	// reset the buffer reader to use the new packet data
	// bufio.NewReader creates a new 16 byte buffer on each call which we want to avoid
	// https://github.com/golang/go/blob/master/src/bufio/bufio.go#L57
	stream.buffer.Reset(bytes.NewReader(data))

	// loop through the parsers until we find one that can parse the request/response
	for _, parser := range stream.parsers {
		success, err := parser.parse(stream, ctx, isClient, stream.buffer, packetCount)
		if err != nil {
			log.Debug().
				Err(err).
				Msg("Error parsing request/response")
		}
		if success {
			break
		}
	}
}

// ReassemblyComplete is called when the TCP assembler believes a stream has completed.
func (stream *tcpStream) ReassemblyComplete(ac reassembly.AssemblerContext) bool {
	log.Debug().
		Str("stream_ident", stream.ident).
		Msg("Connection closed")

	// decrement the number of active streams
	DecrementActiveStreamCount()
	return true // remove the connection, heck with the last ACK
}
