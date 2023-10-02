package assemblers

import (
	"net/http"
	"sync"
	"time"
)

type httpMatcher struct {
	entries *sync.Map
}

type entry struct {
	request             *http.Request
	requestTimestamp    time.Time
	response            *http.Response
	responseTimestamp   time.Time
	requestPacketCount  int
	responsePacketCount int
}

func newRequestResponseMatcher() *httpMatcher {
	return &httpMatcher{
		entries: &sync.Map{},
	}
}

// GetOrStoreRequest receives a tcpStream ident, a timestamp, and a request.
//
// If the response that matches the stream ident has been seen before,
// returns a match entry containing both Request and Response and matchFound will be true.
//
// If the response hasn't been seen yet,
// stores the Request for later lookup and returns match as nil and matchFound will be false.
func (m *httpMatcher) GetOrStoreRequest(key int64, timestamp time.Time, request *http.Request, packetCount int) (match *entry, matchFound bool) {
	e := &entry{
		request:            request,
		requestTimestamp:   timestamp,
		requestPacketCount: packetCount,
	}

	if v, matchFound := m.entries.LoadOrStore(key, e); matchFound {
		m.entries.Delete(key)
		e = v.(*entry) // reuse allocated &entry{} to hold the match
		// found entry has Response, so update it with Request
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
func (m *httpMatcher) GetOrStoreResponse(key int64, timestamp time.Time, response *http.Response, packetCount int) (match *entry, matchFound bool) {
	e := &entry{
		response:            response,
		responseTimestamp:   timestamp,
		responsePacketCount: packetCount,
	}

	if v, matchFound := m.entries.LoadOrStore(key, e); matchFound {
		m.entries.Delete(key)
		e = v.(*entry) // reuse allocated &entry{} to hold the match
		// found entry has Request, so update it with Response
		e.response = response
		e.responseTimestamp = timestamp
		return e, true
	}
	return nil, false
}
