package assemblers

import (
	"bufio"
	"io"
	"net/http"

	"github.com/rs/zerolog/log"
)

type HttpParser struct {
	matcher *httpMatcher
}

func NewHttpParser() *HttpParser {
	return &HttpParser{
		matcher: newRequestResponseMatcher(),
	}
}

func (parser *HttpParser) parse(stream *tcpStream, ctx *Context, isClient bool, buffer *bufio.Reader, packetCount int) (bool, error) {
	if isClient {
		// We use TCP SEQ & ACK numbers to identify request/response pairs
		// ACK corresponds to SEQ of the HTTP response
		// https://madpackets.com/2018/04/25/tcp-sequence-and-acknowledgement-numbers-explained/
		requestId := int64(ctx.ack)
		req, err := http.ReadRequest(buffer)
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			return false, nil
		} else if err != nil {
			log.Debug().
				Err(err).
				Int64("request_id", requestId).
				Str("stream_ident", stream.ident).
				Str("src_ip", stream.srcIP).
				Str("src_port", stream.srcPort).
				Str("dst_ip", stream.dstIP).
				Str("dst_port", stream.dstPort).
				Msg("Error reading HTTP request")
			return false, err
		}
		// We only care about a few headers, so recreate the header with just the ones we need
		req.Header = extractHeaders(req.Header)
		// We don't need the body, so just close it if set
		if req.Body != nil {
			req.Body.Close()
		}
		// get the number of packets that made up this request
		if entry, matchFound := parser.matcher.GetOrStoreRequest(requestId, ctx.CaptureInfo.Timestamp, req, packetCount); matchFound {
			// we have a match, process complete request/response pair
			parser.processEvent(stream, requestId, entry)
		}
	} else {
		// We use TCP SEQ & ACK numbers to identify request/response pairs
		// SEQ corresponds to ACK of the HTTP request
		// https://madpackets.com/2018/04/25/tcp-sequence-and-acknowledgement-numbers-explained/
		requestId := int64(ctx.seq)
		res, err := http.ReadResponse(buffer, nil)
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			return false, nil
		} else if err != nil {
			log.Debug().
				Err(err).
				Int64("request_id", requestId).
				Str("stream_ident", stream.ident).
				Str("src_ip", stream.srcIP).
				Str("src_port", stream.srcPort).
				Str("dst_ip", stream.dstIP).
				Str("dst_port", stream.dstPort).
				Msg("Error reading HTTP response")
			return false, err
		}
		// We only care about a few headers, so recreate the header with just the ones we need
		res.Header = extractHeaders(res.Header)
		// We don't need the body, so just close it if set
		if res.Body != nil {
			res.Body.Close()
		}
		if entry, matchFound := parser.matcher.GetOrStoreResponse(requestId, ctx.CaptureInfo.Timestamp, res, packetCount); matchFound {
			// we have a match, process complete request/response pair
			parser.processEvent(stream, requestId, entry)
		}
	}

	return true, nil
}

func (parser *HttpParser) processEvent(strean *tcpStream, requestId int64, entry *entry) {
	strean.httpEvents <- HttpEvent{
		StreamIdent:         strean.ident,
		RequestId:           requestId,
		Request:             entry.request,
		Response:            entry.response,
		RequestTimestamp:    entry.requestTimestamp,
		ResponseTimestamp:   entry.responseTimestamp,
		RequestPacketCount:  entry.requestPacketCount,
		ResponsePacketCount: entry.responsePacketCount,
		SrcIp:               strean.srcIP,
		DstIp:               strean.dstIP,
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
