package assemblers

import (
	"bufio"
	"net/http"
	"time"
)

// httpParser parses HTTP requests and responses
type httpParser struct {
	matcher *httpMatcher
}

func newHttpParser() *httpParser {
	return &httpParser{
		matcher: newRequestResponseMatcher(),
	}
}

// Parse parses a HTTP request or response and stores it in the matcher
// If a match is found, it sends a HttpEvent to the tcpStream's events channel
func (parser *httpParser) parse(stream *tcpStream, requestId int64, timestamp time.Time, isClient bool, buffer *bufio.Reader, packetCount int) (bool, error) {
	if isClient {
		req, err := http.ReadRequest(buffer)
		if err != nil {
			return false, err
		}
		// We only care about a few headers, so recreate the header with just the ones we need
		req.Header = extractHeaders(req.Header)
		// We don't need the body, so just close it if set
		if req.Body != nil {
			req.Body.Close()
		}
		if entry, matchFound := parser.matcher.GetOrStoreRequest(requestId, timestamp, req, packetCount); matchFound {
			// we have a match, process complete request/response pair
			stream.eventsChan <- NewHttpEvent(
				stream.ident,
				requestId,
				entry.requestTimestamp,
				entry.responseTimestamp,
				entry.requestPacketCount,
				entry.responsePacketCount,
				stream.srcIP,
				stream.dstIP,
				entry.request,
				entry.response,
			)
		}
	} else {
		res, err := http.ReadResponse(buffer, nil)
		if err != nil {
			return false, err
		}
		// We only care about a few headers, so recreate the header with just the ones we need
		res.Header = extractHeaders(res.Header)
		// We don't need the body, so just close it if set
		if res.Body != nil {
			res.Body.Close()
		}
		if entry, matchFound := parser.matcher.GetOrStoreResponse(requestId, timestamp, res, packetCount); matchFound {
			// we have a match, process complete request/response pair
			stream.eventsChan <- NewHttpEvent(
				stream.ident,
				requestId,
				entry.requestTimestamp,
				entry.responseTimestamp,
				entry.requestPacketCount,
				entry.responsePacketCount,
				stream.srcIP,
				stream.dstIP,
				entry.request,
				entry.response,
			)
		}
	}
	return true, nil
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
