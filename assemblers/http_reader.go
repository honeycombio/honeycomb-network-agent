package assemblers

import (
	"bufio"
	"io"
	"net/http"
	"sync"
)

type httpReader struct {
	// ident    string
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
			// body, err := io.ReadAll(req.Body)
			// s := len(body)
			// if err != nil {
			// 	Error("HTTP-request-body", "Got body err: %s\n", err)
			// }
			// req.Body.Close()

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

			// body, err := io.ReadAll(res.Body)
			// s := len(body)
			// if err != nil {
			// 	Error("HTTP-response-body", "HTTP/%s: failed to get body(parsed len:%d): %s\n", h.ident, s, err)
			// }
			// res.Body.Close()

			entry := h.parent.matcher.LoadOrStoreResponse(h.parent.ident, res)
			if entry != nil {
				// we have a match, process complete request/response pair
				h.processEvent(entry)
			}

			if err != nil {
				Error("Error sending even", "error sending event: %e\n", err)
			}

			// Do we care about response decoding right now?
			// encoding := res.Header["Content-Encoding"]
			// if err == nil {
			// 	base := url.QueryEscape(path.Base(eventAttrs["http.request_url"]))
			// 	if err != nil {
			// 		base = "incomplete-" + base
			// 	}
			// 	if len(base) > 250 {
			// 		base = base[:250] + "..."
			// 	}
			// 	target := base
			// 	n := 0
			// 	for true {
			// 		_, err := os.Stat(target)
			// 		//if os.IsNotExist(err) != nil {
			// 		if err != nil {
			// 			break
			// 		}
			// 		target = fmt.Sprintf("%s-%d", base, n)
			// 		n++
			// 	}
			// 	f, err := os.Create(target)
			// 	if err != nil {
			// 		Error("HTTP-create", "Cannot create %s: %s\n", target, err)
			// 		continue
			// 	}
			// 	var r io.Reader
			// 	r = bytes.NewBuffer(body)
			// 	if len(encoding) > 0 && (encoding[0] == "gzip" || encoding[0] == "deflate") {
			// 		r, err = gzip.NewReader(r)
			// 		if err != nil {
			// 			Error("HTTP-gunzip", "Failed to gzip decode: %s", err)
			// 		}
			// 	}
			// 	if err == nil {
			// 		w, err := io.Copy(f, r)
			// 		if _, ok := r.(*gzip.Reader); ok {
			// 			r.(*gzip.Reader).Close()
			// 		}
			// 		f.Close()
			// 		if err != nil {
			// 			Error("HTTP-save", "%s: failed to save %s (l:%d): %s\n", h.ident, target, w, err)
			// 		} else {
			// 			Info("%s: Saved %s (l:%d)\n", h.ident, target, w)
			// 		}
			// 	}
			// }
		}
	}
}

func (h *httpReader) processEvent(entry *entry) {
	h.parent.events <- httpEvent{
		requestId: h.parent.ident,
		request: entry.request,
		response: entry.response,
		duration: entry.responseTimestamp.Sub(entry.requestTimestamp),
	}
}