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
// If the response that matches the stream ident has been seen before,
// returns a match entry containing both Request and Response and matchFound will be true.
//
// If the response hasn't been seen yet,
// stores the Request for later lookup and returns match as nil and matchFound will be false.
func (m *httpMatcher) GetOrStoreRequest(ident string, timestamp time.Time, request *http.Request) (match *entry, matchFound bool) {
	e := &entry{
		request:          request,
		requestTimestamp: timestamp,
	}
	if v, loaded := m.messages.LoadOrStore(ident, e); loaded {
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
// If the request that matches the stream ident has been seen before,
// returns a match entry containing both Request and Response and matchFound will be true.
//
// If the request hasn't been seen yet,
// stores the Response for later lookup and returns match as nil and matchFound will be false.
func (m *httpMatcher) GetOrStoreResponse(ident string, timestamp time.Time, response *http.Response) (match *entry, matchFound bool) {
	e := &entry{
		response:          response,
		responseTimestamp: timestamp,
	}
	if v, loaded := m.messages.LoadOrStore(ident, e); loaded {
		m.messages.Delete(ident)
		e = v.(*entry)
		e.response = response
		e.responseTimestamp = timestamp
		return e, true
	}
	return nil, false
}
