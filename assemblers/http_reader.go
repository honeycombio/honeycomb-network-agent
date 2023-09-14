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
	// Seq will hold SEQ or ACK number for incoming or outgoing HTTP TCP segments
	// https://madpackets.com/2018/04/25/tcp-sequence-and-acknowledgement-numbers-explained/
	Seq int
}

type httpReader struct {
	isClient  bool
	srcIp     string
	srcPort   string
	dstIp     string
	dstPort   string
	data      []byte
	parent    *tcpStream
	messages  chan message
	timestamp time.Time
	seq       int
}

func (h *httpReader) Read(p []byte) (int, error) {
	var msg message
	ok := true
	for ok && len(h.data) == 0 {
		msg, ok = <-h.messages
		h.timestamp = msg.timestamp
		h.seq = msg.Seq
		h.data = msg.data
		msg.data = nil // clear the []byte so we can release the memory
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

			ident := fmt.Sprintf("%s:%d", h.parent.ident, h.seq)
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

			ident := fmt.Sprintf("%s:%d", h.parent.ident, h.seq)
			if entry, ok := h.parent.matcher.GetOrStoreResponse(ident, h.timestamp, res); ok {
				// we have a match, process complete request/response pair
				h.processEvent(ident, entry)
			}
		}
	}
}

func (h *httpReader) processEvent(ident string, entry *entry) {
	h.parent.events <- HttpEvent{
		RequestId:         ident,
		Request:           entry.request,
		Response:          entry.response,
		RequestTimestamp:  entry.requestTimestamp,
		ResponseTimestamp: entry.responseTimestamp,
		SrcIp:             h.srcIp,
		DstIp:             h.dstIp,
	}
}

func (h *httpReader) close() error {
	close(h.messages)
	h.data = nil // release the data, free up that memory! ᕕ( ᐛ )ᕗ
	return nil
}
