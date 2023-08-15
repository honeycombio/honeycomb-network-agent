package assemblers

import (
	"bufio"
	"io"
	"net/http"
	"sync"
)

type httpReader struct {
	isClient bool
	srcIp    string
	srcPort  string
	dstIp    string
	dstPort  string
	bytes    chan []byte
	data     []byte
	parent   *tcpStream
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
	for true {
		if h.isClient {
			req, err := http.ReadRequest(b)
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				break
			} else if err != nil {
				// Error("HTTP-request", "HTTP/%s Request error: %s (%v,%+v)\n", h.ident, err, err, err)
				continue
			}
			entry := h.parent.matcher.LoadOrStoreRequest(h.parent.ident, req)
			if entry != nil {
				// we have a match, process complete request/response pair
				h.processEvent(entry)
			}
		} else {
			res, err := http.ReadResponse(b, nil)
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				break
			} else if err != nil {
				// Error("HTTP-response", "HTTP/%s Response error: %s (%v,%+v)\n", h.ident, err, err, err)
				continue
			}

			entry := h.parent.matcher.LoadOrStoreResponse(h.parent.ident, res)
			if entry != nil {
				// we have a match, process complete request/response pair
				h.processEvent(entry)
			}
		}
	}
}

func (h *httpReader) processEvent(entry *entry) {
	h.parent.events <- httpEvent{
		requestId: h.parent.ident,
		request:   entry.request,
		response:  entry.response,
		duration:  entry.responseTimestamp.Sub(entry.requestTimestamp),
		srcIp:     h.srcIp,
		dstIp:     h.dstIp,
	}
}
