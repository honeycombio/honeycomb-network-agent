package assemblers

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"sync"

	"github.com/honeycombio/libhoney-go"
)

type httpReader struct {
	ident    string
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
				Error("HTTP-request", "HTTP/%s Request error: %s (%v,%+v)\n", h.ident, err, err, err)
				continue
			}
			body, err := io.ReadAll(req.Body)
			s := len(body)
			if err != nil {
				Error("HTTP-request-body", "Got body err: %s\n", err)
			}
			req.Body.Close()

			eventAttrs := map[string]string{
				"name":                     fmt.Sprintf("HTTP %s", req.Method),
				"http.request_method":      req.Method,
				"http.request_ident":       h.ident,
				"http.request_source_ip":   h.srcIp,
				"http.request_source_port": h.srcPort,
				"http.request_dest_ip":     h.dstIp,
				"http.request_dest_port":   h.dstPort,
				"http.request_url":         req.RequestURI,
				"http.request_body":        fmt.Sprintf("%v", req.Body),
				"http.request_headers":     fmt.Sprintf("%v", req.Header),
				"http.h_request_bytes":     string(<-h.bytes),
			}

			Info("HTTP/%s Request: %s %s (body:%d)\n", h.ident, req.Method, req.URL, s)
			h.parent.Lock()
			h.parent.urls = append(h.parent.urls, req.URL.String())
			h.parent.eventAttrs = eventAttrs
			h.parent.Unlock()
		} else {
			res, err := http.ReadResponse(b, nil)
			var req string
			var eventAttrs map[string]string
			h.parent.Lock()
			if len(h.parent.urls) == 0 {
				req = fmt.Sprintf("<no-request-seen>")
			} else {
				req, h.parent.urls = h.parent.urls[0], h.parent.urls[1:]
				eventAttrs = h.parent.eventAttrs
			}
			h.parent.Unlock()
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				break
			} else if err != nil {
				Error("HTTP-response", "HTTP/%s Response error: %s (%v,%+v)\n", h.ident, err, err, err)
				continue
			}

			body, err := io.ReadAll(res.Body)
			s := len(body)
			if err != nil {
				Error("HTTP-response-body", "HTTP/%s: failed to get body(parsed len:%d): %s\n", h.ident, s, err)
			}
			res.Body.Close()

			ev := libhoney.NewEvent()
			ev.Add(eventAttrs)
			ev.AddField("http.response_ident", h.ident)
			ev.AddField("http.response_body", res.Body)
			ev.AddField("http.response_code", res.StatusCode)
			ev.AddField("http.response_headers", res.Header)
			ev.AddField("http.h_response_bytes", h.bytes)
			ev.AddField("http.response_request_url", req)

			err = ev.Send()
			if err != nil {
				Error("Error sending even", "error sending event: %e\n", err)
			}

			sym := ","
			if res.ContentLength > 0 && res.ContentLength != int64(s) {
				sym = "!="
			}
			contentType, ok := res.Header["Content-Type"]
			if !ok {
				contentType = []string{http.DetectContentType(body)}
			}
			encoding := res.Header["Content-Encoding"]
			Info("HTTP/%s Response: %s URL:%s (%d%s%d%s) -> %s\n", h.ident, res.Status, req, res.ContentLength, sym, s, contentType, encoding)
			if err == nil {
				base := url.QueryEscape(path.Base(req))
				if err != nil {
					base = "incomplete-" + base
				}
				if len(base) > 250 {
					base = base[:250] + "..."
				}
				target := base
				n := 0
				for true {
					_, err := os.Stat(target)
					//if os.IsNotExist(err) != nil {
					if err != nil {
						break
					}
					target = fmt.Sprintf("%s-%d", base, n)
					n++
				}
				f, err := os.Create(target)
				if err != nil {
					Error("HTTP-create", "Cannot create %s: %s\n", target, err)
					continue
				}
				var r io.Reader
				r = bytes.NewBuffer(body)
				if len(encoding) > 0 && (encoding[0] == "gzip" || encoding[0] == "deflate") {
					r, err = gzip.NewReader(r)
					if err != nil {
						Error("HTTP-gunzip", "Failed to gzip decode: %s", err)
					}
				}
				if err == nil {
					w, err := io.Copy(f, r)
					if _, ok := r.(*gzip.Reader); ok {
						r.(*gzip.Reader).Close()
					}
					f.Close()
					if err != nil {
						Error("HTTP-save", "%s: failed to save %s (l:%d): %s\n", h.ident, target, w, err)
					} else {
						Info("%s: Saved %s (l:%d)\n", h.ident, target, w)
					}
				}
			}
		}
	}
}
