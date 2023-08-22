package assemblers

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

type httpReader struct {
	isClient  bool
	srcIp     string
	srcPort   string
	dstIp     string
	dstPort   string
	bytes     chan []byte
	data      []byte
	parent    *tcpStream
	timestamp time.Time
}

func (h *httpReader) Read(p []byte) (int, error) {
	ok := true
	for ok && len(h.data) == 0 {
		h.data, ok = <-h.bytes
	}
	if !ok || len(h.data) == 0 {
		return 0, io.EOF
	}

	l := copy(p, h.data)
	h.data = h.data[l:]
	return l, nil
}

func (h *httpReader) run(wg *sync.WaitGroup) {
	defer wg.Done()
	b := bufio.NewReader(h)
	for {
		if h.isClient {
			req, err := http.ReadRequest(b)
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				break
			} else if err != nil {
				continue
			}
			requestCount := h.parent.counter.incrementRequest()
			ident := fmt.Sprintf("%s:%d", h.parent.ident, requestCount)
			// log.Info().
			// 	Str("ident", ident).
			// 	Msg("Storing request")
			entry := h.parent.matcher.LoadOrStoreRequest(ident, h.timestamp, req)
			if entry != nil {
				// we have a match, process complete request/response pair
				h.processEvent(ident, entry)
			}
		} else {
			res, err := http.ReadResponse(b, nil)
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				break
			} else if err != nil {
				// Error("HTTP-response", "HTTP/%s Response error: %s (%v,%+v)\n", h.ident, err, err, err)
				continue
			}

			responseCount := h.parent.counter.incrementResponse()
			ident := fmt.Sprintf("%s:%d", h.parent.ident, responseCount)
			// log.Info().
			// 	Str("ident", ident).
			// 	Msg("Storing response")
			entry := h.parent.matcher.LoadOrStoreResponse(ident, h.timestamp, res)
			if entry != nil {
				// we have a match, process complete request/response pair
				h.processEvent(ident, entry)
			}
		}
	}
}

func (h *httpReader) processEvent(ident string, entry *entry) {
	// log.Info().
	// 	Str("ident", ident).
	// 	Msg("Found match")
	h.parent.events <- HttpEvent{
		RequestId: ident,
		Request:   entry.request,
		Response:  entry.response,
		Timestamp: entry.requestTimestamp,
		Duration:  entry.responseTimestamp.Sub(entry.requestTimestamp),
		SrcIp:     h.srcIp,
		DstIp:     h.dstIp,
	}
}
