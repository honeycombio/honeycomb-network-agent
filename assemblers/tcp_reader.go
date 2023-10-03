package assemblers

import (
	"bufio"
	"bytes"
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
		requestId := int64(ctx.ack)
		req, err := http.ReadRequest(reader.buffer)
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			return
		} else if err != nil {
			log.Debug().
				Err(err).
				Int64("request_id", requestId).
				Str("stream_ident", reader.streamIdent).
				Str("src_ip", reader.srcIp).
				Str("src_port", reader.srcPort).
				Str("dst_ip", reader.dstIp).
				Str("dst_port", reader.dstPort).
				Msg("Error reading HTTP request")
			return
		}
		// We only care about a few headers, so recreate the header with just the ones we need
		req.Header = extractHeaders(req.Header)
		// We don't need the body, so just close it if set
		if req.Body != nil {
			req.Body.Close()
		}
		// get the number of packets that made up this request
		packetCount := sg.Stats().Packets
		if entry, matchFound := reader.matcher.GetOrStoreRequest(requestId, ctx.CaptureInfo.Timestamp, req, packetCount); matchFound {
			// we have a match, process complete request/response pair
			reader.processEvent(requestId, entry)
		}
	} else {
		// We use TCP SEQ & ACK numbers to identify request/response pairs
		// SEQ corresponds to ACK of the HTTP request
		// https://madpackets.com/2018/04/25/tcp-sequence-and-acknowledgement-numbers-explained/
		requestId := int64(ctx.seq)
		res, err := http.ReadResponse(reader.buffer, nil)
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			return
		} else if err != nil {
			log.Debug().
				Err(err).
				Int64("request_id", requestId).
				Str("stream_ident", reader.streamIdent).
				Str("src_ip", reader.srcIp).
				Str("src_port", reader.srcPort).
				Str("dst_ip", reader.dstIp).
				Str("dst_port", reader.dstPort).
				Msg("Error reading HTTP response")
			return
		}
		// We only care about a few headers, so recreate the header with just the ones we need
		res.Header = extractHeaders(res.Header)
		// We don't need the body, so just close it if set
		if res.Body != nil {
			res.Body.Close()
		}
		// get the number of packets that made up this request
		packetCount := sg.Stats().Packets
		if entry, matchFound := reader.matcher.GetOrStoreResponse(requestId, ctx.CaptureInfo.Timestamp, res, packetCount); matchFound {
			// we have a match, process complete request/response pair
			reader.processEvent(requestId, entry)
		}
	}
}

func (reader *tcpReader) processEvent(requestId int64, entry *entry) {
	reader.events <- HttpEvent{
		StreamIdent:         reader.streamIdent,
		RequestId:           requestId,
		Request:             entry.request,
		Response:            entry.response,
		RequestTimestamp:    entry.requestTimestamp,
		ResponseTimestamp:   entry.responseTimestamp,
		RequestPacketCount:  entry.requestPacketCount,
		ResponsePacketCount: entry.responsePacketCount,
		SrcIp:               reader.srcIp,
		DstIp:               reader.dstIp,
	}
}

var headersToExtract = []string{
	"User-Agent",
}

// extractHeaders returns a new http.Header object with only specified headers from the original.
// The original request/response header contains a lot of stuff we don't really care about
// and stays in memory until the request/response pair is processed
func extractHeaders(header http.Header) http.Header {
	cleanHeader := http.Header{}
	if header == nil {
		return cleanHeader
	}
	for _, headerName := range headersToExtract {
		if headerValue := header.Get(headerName); headerValue != "" {
			cleanHeader.Set(headerName, headerValue)
		}
	}
	return cleanHeader
}
