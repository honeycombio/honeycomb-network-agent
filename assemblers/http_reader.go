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

func (h *httpReader) Read(p []byte) (int, error) {
	var msg message
	ok := true
	for ok && len(h.data) == 0 {
		msg, ok = <-h.messages
		h.data = msg.data
		h.timestamp = msg.timestamp
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
	for {
		b := bufio.NewReader(h)
		if h.isClient {
			req, err := http.ReadRequest(b)
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				break
			} else if err != nil {
				log.Debug().
					Err(err).
					Str("ident", h.parent.ident).
					Msg("Error reading HTTP request")
				continue
			}

			requestCount := h.parent.counter.incrementRequest()
			ident := fmt.Sprintf("%s:%d", h.parent.ident, requestCount)
			if entry, ok := h.parent.matcher.GetOrStoreRequest(ident, h.timestamp, req); ok {
				// we have a match, process complete request/response pair
				h.processEvent(ident, entry)
			}
		} else {
			res, err := http.ReadResponse(b, nil)
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				break
			} else if err != nil {
				log.Debug().
					Err(err).
					Str("ident", h.parent.ident).
					Msg("Error reading HTTP response")
				continue
			}

			responseCount := h.parent.counter.incrementResponse()
			ident := fmt.Sprintf("%s:%d", h.parent.ident, responseCount)
			if entry, ok := h.parent.matcher.GetOrStoreResponse(ident, h.timestamp, res); ok {
				// we have a match, process complete request/response pair
				h.processEvent(ident, entry)
			}
		}
	}
}

func (h *httpReader) processEvent(ident string, entry *entry) {
	eventDuration := entry.responseTimestamp.Sub(entry.requestTimestamp)
	if eventDuration < 0 { // the response came in before the request? wat?
		// logging the weirdness for now so we can debug in environments with production loads
		log.Debug().
			Str("ident", ident).
			Int64("duration_ns", int64(eventDuration)).
			Int64("duration_ms", eventDuration.Milliseconds()).
			Msg("Time has gotten weird for this event.")
	}

	h.parent.events <- HttpEvent{
		RequestId: ident,
		Request:   entry.request,
		Response:  entry.response,
		Timestamp: entry.requestTimestamp,
		Duration:  eventDuration,
		SrcIp:     h.srcIp,
		DstIp:     h.dstIp,
	}
}
