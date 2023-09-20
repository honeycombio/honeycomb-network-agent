package assemblers

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net/http"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/reassembly"
	"github.com/rs/zerolog/log"
)

type tcpReader struct {
	streamIdent string
	isClient    bool
	srcIp       string
	srcPort     string
	dstIp       string
	dstPort     string
	matcher     *httpMatcher
	events      chan HttpEvent
	buffer      *bufio.Reader
}

func NewTcpReader(streamIdent string, isClient bool, net gopacket.Flow, transport gopacket.Flow, matcher *httpMatcher, httpEvents chan HttpEvent) *tcpReader {
	return &tcpReader{
		streamIdent: streamIdent,
		isClient:    isClient,
		srcIp:       net.Src().String(),
		dstIp:       net.Dst().String(),
		srcPort:     transport.Src().String(),
		dstPort:     transport.Dst().String(),
		matcher:     matcher,
		events:      httpEvents,
		buffer:      bufio.NewReader(bytes.NewReader(nil)),
	}
}

func (reader *tcpReader) reassembledSG(sg reassembly.ScatterGather, ac reassembly.AssemblerContext) {
	len, _ := sg.Lengths()
	data := sg.Fetch(len)
	ctx, ok := ac.(*Context)
	if !ok {
		log.Warn().
			Msg("Failed to cast ScatterGather to ContextWithSeq")
	}

	// reset the buffer reader to use the new packet data
	// bufio.NewReader creates a new 16 byte buffer on each call which we want to avoid
	// https://github.com/golang/go/blob/master/src/bufio/bufio.go#L57
	reader.buffer.Reset(bytes.NewReader(data))
	if reader.isClient {
		// We use TCP SEQ & ACK numbers to identify request/response pairs
		// ACK corresponds to SEQ of the HTTP response
		// https://madpackets.com/2018/04/25/tcp-sequence-and-acknowledgement-numbers-explained/
		reqIdent := fmt.Sprintf("%s:%d", reader.streamIdent, ctx.ack)
		req, err := http.ReadRequest(reader.buffer)
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			return
		} else if err != nil {
			log.Debug().
				Err(err).
				Str("ident", reader.streamIdent).
				Msg("Error reading HTTP request")
			return
		}
		// We don't need the body, so just close it if set
		if req.Body != nil {
			req.Body.Close()
		}

		if entry, matchFound := reader.matcher.GetOrStoreRequest(reqIdent, ctx.CaptureInfo.Timestamp, req); matchFound {
			// we have a match, process complete request/response pair
			reader.processEvent(reqIdent, entry)
		}
	} else {
		// We use TCP SEQ & ACK numbers to identify request/response pairs
		// SEQ corresponds to ACK of the HTTP request
		// https://madpackets.com/2018/04/25/tcp-sequence-and-acknowledgement-numbers-explained/
		resIdent := fmt.Sprintf("%s:%d", reader.streamIdent, ctx.seq)
		res, err := http.ReadResponse(reader.buffer, nil)
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			return
		} else if err != nil {
			log.Debug().
				Err(err).
				Str("ident", resIdent).
				Msg("Error reading HTTP response")
			return
		}
		// We don't need the body, so just close it if set
		if res.Body != nil {
			res.Body.Close()
		}

		if entry, matchFound := reader.matcher.GetOrStoreResponse(resIdent, ctx.CaptureInfo.Timestamp, res); matchFound {
			// we have a match, process complete request/response pair
			reader.processEvent(resIdent, entry)
		}
	}
}

func (reader *tcpReader) processEvent(ident string, entry *entry) {
	reader.events <- HttpEvent{
		RequestId:         ident,
		Request:           entry.request,
		Response:          entry.response,
		RequestTimestamp:  entry.requestTimestamp,
		ResponseTimestamp: entry.responseTimestamp,
		SrcIp:             reader.srcIp,
		DstIp:             reader.dstIp,
	}
}
