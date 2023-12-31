package assemblers

import (
	"bufio"
	"bytes"
	"fmt"
	"io"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/reassembly"
	"github.com/rs/zerolog/log"

	"github.com/honeycombio/honeycomb-network-agent/config"
)

// tcpStream represents a TCP stream and receives TCP packets from the gopacket assembler
// and attempts to parses them into requests and responses
//
// It implements gopacket's reassembly.Stream interface
type tcpStream struct {
	id         uint64
	ident      string
	config     config.Config
	tcpstate   *reassembly.TCPSimpleFSM
	fsmerr     bool
	optchecker reassembly.TCPOptionCheck
	eventsChan chan Event
	srcIP      string
	dstIP      string
	srcPort    string
	dstPort    string
	buffer     *bufio.Reader
	parsers    []parser
}

func NewTcpStream(net gopacket.Flow, transport gopacket.Flow, config config.Config, eventsChan chan Event) *tcpStream {
	streamId := IncrementStreamCount()
	return &tcpStream{
		id:     streamId,
		ident:  fmt.Sprintf("%s:%s:%d", net, transport, streamId),
		config: config,
		tcpstate: reassembly.NewTCPSimpleFSM(reassembly.TCPSimpleFSMOptions{
			SupportMissingEstablishment: true,
		}),
		fsmerr:     false, // TODO: verify whether we need this
		optchecker: reassembly.NewTCPOptionCheck(),
		eventsChan: eventsChan,
		srcIP:      net.Src().String(),
		dstIP:      net.Dst().String(),
		srcPort:    transport.Src().String(),
		dstPort:    transport.Dst().String(),
		buffer:     bufio.NewReader(bytes.NewReader(nil)),
		parsers: []parser{
			newHttpParser(config.HTTPHeadersToExtract),
		},
	}
}

// Accept implements gopacket's [reassembly.Stream.Accept] interface.
func (stream *tcpStream) Accept(tcp *layers.TCP, ci gopacket.CaptureInfo, dir reassembly.TCPFlowDirection, nextSeq reassembly.Sequence, start *bool, ac reassembly.AssemblerContext) bool {
	// FSM
	if !stream.tcpstate.CheckState(tcp, dir) {
		// Error("FSM", "%s: Packet rejected by FSM (state:%s)\n", t.ident, t.tcpstate.String())
		stats.rejectFsm.Add(1)
		if !stream.fsmerr {
			stream.fsmerr = true
			stats.rejectConnFsm.Add(1)
		}
		if !stream.config.Ignorefsmerr {
			return false
		}
	}
	// Options
	err := stream.optchecker.Accept(tcp, ci, dir, nextSeq, start)
	if err != nil {
		// Error("OptionChecker", "%s: Packet rejected by OptionChecker: %s\n", t.ident, err)
		stats.rejectOpt.Add(1)
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
		stats.rejectOpt.Add(1)
	}
	return accept
}

// ReassembledSG implements gopacket's [reassembly.Stream.ReassembledSG] interface.
// This is where most of the work happens.
//
// Note: the reassembly.ScatterGather param (sg) is reused after each ReassembledSG call, so copy what's needed out of it before return.
func (stream *tcpStream) ReassembledSG(sg reassembly.ScatterGather, ac reassembly.AssemblerContext) {
	// Get the direction of the packet (client to server or server to client)
	dir, _, _, _ := sg.Info()
	isClient := dir == reassembly.TCPDirClientToServer

	// get our custom context that includes the TCP seq/ack numbers
	ctx, ok := ac.(*Context)
	if !ok {
		log.Warn().
			Msg("Failed to cast given AssemblerContext to ContextWithSeq")
	}

	// We use TCP SEQ & ACK numbers to identify request/response pairs
	// A request's ACK corresponds to the SEQ of it's matching response
	// https://madpackets.com/2018/04/25/tcp-sequence-and-acknowledgement-numbers-explained/
	var requestId int64
	if isClient {
		requestId = int64(ctx.ack)
	} else {
		requestId = int64(ctx.seq)
	}

	// get the number of packets that made up this request
	packetCount := sg.Stats().Packets

	// get the data from the packet
	len, _ := sg.Lengths()
	data := sg.Fetch(len)

	// reset the buffer reader to use the new packet data
	// bufio.NewReader creates a new 16 byte buffer on each call,
	// so we reset the existing buffer with those bytes instead of
	// allocating new memory
	// https://github.com/golang/go/blob/master/src/bufio/bufio.go#L57
	stream.buffer.Reset(bytes.NewReader(data))

	// loop through the parsers until we find one that can parse the request/response
	for _, parser := range stream.parsers {
		success, err := parser.parse(stream, requestId, ctx.CaptureInfo.Timestamp, isClient, stream.buffer, packetCount)
		if err != nil {
			// if we hit the end of the stream, stop trying to parse
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				break
			}
			log.Debug().
				Err(err).
				Int64("request_id", requestId).
				Str("stream_ident", stream.ident).
				Str("src_ip", stream.srcIP).
				Str("src_port", stream.srcPort).
				Str("dst_ip", stream.dstIP).
				Str("dst_port", stream.dstPort).
				Msg("Error parsing packet")
			continue
		}
		if success {
			break
		}
	}
}

// ReassemblyComplete implements gopacket's [reassembly.Stream.ReassemblyComplete] interface.
// Called when gopacket's assembly internals has decided that there is no more data for this Stream
// (e.g. FIN or RST packet, timed out without new data).
//
// Our implementation always returns true to remove the connection from the gopacket-managed pool.
// We don't return false, because we aren't interested in the last FIN-ACK.
func (stream *tcpStream) ReassemblyComplete(ac reassembly.AssemblerContext) bool {
	log.Debug().
		Str("stream_ident", stream.ident).
		Msg("Connection closed")
	DecrementActiveStreamCount()
	return true // remove the connection, heck with the last ACK
}
