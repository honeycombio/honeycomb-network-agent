package assemblers

import (
	"net/http"
	"sync"
	"time"
)

type httpMatcher struct {
	messages *sync.Map
}

type entry struct {
	request           *http.Request
	requestTimestamp  time.Time
	response          *http.Response
	responseTimestamp time.Time
}

func newRequestResponseMatcher() *httpMatcher {
	return &httpMatcher{
		messages: &sync.Map{},
	}
}

// GetOrStoreRequest receives a tcpStream ident, a timestamp, and a request.
//
// If the response that matches the stream ident has been seen before, return an entry with both Request and Response.
//
// If the response hasn't been seen yet, store the Request for later lookup.
func (m *httpMatcher) GetOrStoreRequest(ident string, timestamp time.Time, request *http.Request) (*entry, bool) {
	e := &entry{
		request:          request,
		requestTimestamp: timestamp,
	}
	if v, ok := m.messages.LoadOrStore(ident, e); ok {
		m.messages.Delete(ident)
		e = v.(*entry)
		e.request = request
		e.requestTimestamp = timestamp
		return e, true
	}
	return nil, false
}

// GetOrStoreResponse receives a tcpStream ident, a timestamp, and a response.
//
// If the request that matches the stream ident has been seen before, return an entry with both Request and Response.
//
// If the request hasn't been seen yet, store the Response for later lookup.
func (m *httpMatcher) GetOrStoreResponse(ident string, timestamp time.Time, response *http.Response) (*entry, bool) {
	e := &entry{
		response:          response,
		responseTimestamp: timestamp,
	}
	if v, ok := m.messages.LoadOrStore(ident, e); ok {
		m.messages.Delete(ident)
		e = v.(*entry)
		e.response = response
		e.responseTimestamp = timestamp
		return e, true
	}
	return nil, false
}
