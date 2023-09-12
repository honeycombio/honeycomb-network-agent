package assemblers

import (
	"fmt"
	"sync"

	"github.com/honeycombio/ebpf-agent/config"
	"github.com/honeycombio/gopacket"
	"github.com/honeycombio/gopacket/layers"
	"github.com/honeycombio/gopacket/reassembly"
	"github.com/rs/zerolog/log"
)

// tcpStream has two unidirectional httpReaders, one for client and one for server
type tcpStream struct {
	id             uint64
	tcpstate       *reassembly.TCPSimpleFSM
	fsmerr         bool
	optchecker     reassembly.TCPOptionCheck
	net, transport gopacket.Flow
	client         httpReader
	server         httpReader
	ident          string
	closed         bool
	config         config.Config
	sync.Mutex
	matcher httpMatcher
	events  chan HttpEvent
}

func (t *tcpStream) Accept(tcp *layers.TCP, ci gopacket.CaptureInfo, dir reassembly.TCPFlowDirection, nextSeq reassembly.Sequence, start *bool, ac reassembly.AssemblerContext) bool {
	// FSM
	if !t.tcpstate.CheckState(tcp, dir) {
		// Error("FSM", "%s: Packet rejected by FSM (state:%s)\n", t.ident, t.tcpstate.String())
		stats.rejectFsm++
		if !t.fsmerr {
			t.fsmerr = true
			stats.rejectConnFsm++
		}
		if !t.config.Ignorefsmerr {
			return false
		}
	}
	// Options
	err := t.optchecker.Accept(tcp, ci, dir, nextSeq, start)
	if err != nil {
		// Error("OptionChecker", "%s: Packet rejected by OptionChecker: %s\n", t.ident, err)
		stats.rejectOpt++
		if !t.config.Nooptcheck {
			return false
		}
	}
	// Checksum
	accept := true
	if t.config.Checksum {
		c, err := tcp.ComputeChecksum()
		if err != nil {
			log.Error().
				Err(err).
				Str("tcp_stream_ident", t.ident).
				Msg("ChecksumCompute")
			accept = false
		} else if c != 0x0 {
			log.Error().
				Str("tcp_stream_ident", t.ident).
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

func (t *tcpStream) ReassembledSG(sg reassembly.ScatterGather, ac reassembly.AssemblerContext) {
	dir, start, end, skip := sg.Info()
	length, saved := sg.Lengths()
	// update stats
	sgStats := sg.Stats()
	if skip > 0 {
		stats.missedBytes += skip
	}
	stats.sz += length - saved
	stats.pkt += sgStats.Packets
	if sgStats.Chunks > 1 {
		stats.reassembled++
	}
	stats.outOfOrderPackets += sgStats.QueuedPackets
	stats.outOfOrderBytes += sgStats.QueuedBytes
	if length > stats.biggestChunkBytes {
		stats.biggestChunkBytes = length
	}
	if sgStats.Packets > stats.biggestChunkPackets {
		stats.biggestChunkPackets = sgStats.Packets
	}
	if sgStats.OverlapBytes != 0 && sgStats.OverlapPackets == 0 {
		log.Fatal().
			Int("bytes", sgStats.OverlapBytes).
			Int("packets", sgStats.OverlapPackets).
			Msg("Invalid overlap")
		panic("Invalid overlap")
	}
	stats.overlapBytes += sgStats.OverlapBytes
	stats.overlapPackets += sgStats.OverlapPackets

	var ident string
	if dir == reassembly.TCPDirClientToServer {
		ident = fmt.Sprintf("%v %v", t.net, t.transport)
	} else {
		ident = fmt.Sprintf("%v %v", t.net.Reverse(), t.transport.Reverse())
	}
	log.Debug().
		Str("ident", ident).            // ex: "192.168.65.4->192.168.65.4 6443->38304"
		Str("direction", dir.String()). // ex: "client->server" or "server->client"
		Int("byte_count", length).
		Bool("start", start).
		Bool("end", end).
		Int("skip", skip).
		Int("saved", saved).
		Int("packet_count", sgStats.Packets).
		Int("chunk_count", sgStats.Chunks).
		Int("overlap_byte_count", sgStats.OverlapBytes).
		Int("overlap_packet_count", sgStats.OverlapPackets).
		Msg("SG reassembled packet")
	if skip == -1 && t.config.Allowmissinginit {
		// this is allowed
	} else if skip != 0 {
		// Missing bytes in stream: do not even try to parse it
		return
	}

	ctx, ok := ac.(*Context)
	if !ok {
		log.Warn().
			Msg("Failed to cast ScatterGather to ContextWithSeq")
	}

	if length > 0 {
		data := sg.Fetch(length)
		if dir == reassembly.TCPDirClientToServer {
			t.client.messages <- message{
				data:      data,
				timestamp: ctx.CaptureInfo.Timestamp,
				Seq:       int(ctx.ack), // client ACK matches server SEQ
			}
		} else {
			t.server.messages <- message{
				data:      data,
				timestamp: ctx.CaptureInfo.Timestamp,
				Seq:       int(ctx.seq), // server SEQ matches client ACK
			}
		}
	}
}

func (t *tcpStream) ReassemblyComplete(ac reassembly.AssemblerContext) bool {
	log.Debug().
		Str("tcp_stream_ident", t.ident).
		Msg("Connection closed")
	t.close()
	// do not remove the connection to allow last ACK
	return false
}

func (t *tcpStream) close() {
	t.Lock()
	defer t.Unlock()

	if !t.closed {
		t.closed = true
		t.client.close()
		t.server.close()
	}
}
