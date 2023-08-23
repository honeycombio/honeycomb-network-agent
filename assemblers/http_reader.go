package assemblers

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

type message struct {
	data      []byte
	timestamp time.Time
}

type httpReader struct {
	isClient  bool
	srcIp     string
	srcPort   string
	dstIp     string
	dstPort   string
	bytes     chan []byte
	data      []byte
	parent    *tcpStream
	messages  chan message
	timestamp time.Time
}

func (reader *httpReader) Read(p []byte) (int, error) {
	var msg message
	ok := true
	for ok && len(reader.data) == 0 {
		msg, ok = <-reader.messages
		reader.data = msg.data
		reader.timestamp = msg.timestamp
	}
	if !ok || len(reader.data) == 0 {
		return 0, io.EOF
	}

	l := copy(p, reader.data)
	reader.data = reader.data[l:]
	return l, nil
}

func (h *httpReader) run(wg *sync.WaitGroup) {
	defer wg.Done()
	for {
		b := bufio.NewReader(h)
		if h.isClient {
			req, err := http.ReadRequest(b)
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				break
			} else if err != nil {
				log.Warn().
					Err(err).
					Str("ident", h.parent.ident).
					Msg("Error reading HTTP request")
				continue
			}
			requestCount := h.parent.counter.incrementRequest()
			ident := fmt.Sprintf("%s:%d", h.parent.ident, requestCount)
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
				log.Warn().
					Err(err).
					Str("ident", h.parent.ident).
					Msg("Error reading HTTP response")
				continue
			}

			responseCount := h.parent.counter.incrementResponse()
			ident := fmt.Sprintf("%s:%d", h.parent.ident, responseCount)
			entry := h.parent.matcher.LoadOrStoreResponse(ident, h.timestamp, res)
			if entry != nil {
				// we have a match, process complete request/response pair
				h.processEvent(ident, entry)
			}
		}
	}
}

func (h *httpReader) processEvent(ident string, entry *entry) {
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

func (r *httpReader) sendMessage(message message) {
	// TODO: check not closed?
	r.messages <- message
}
