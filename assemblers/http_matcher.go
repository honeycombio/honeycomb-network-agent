package assemblers

import (
	"net/http"
	"sync"
	"time"
)

type httpMatcher struct {
	messages map[int64]*entry
	mtx      *sync.Mutex
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
		messages: make(map[int64]*entry),
		mtx:      &sync.Mutex{},
	}
}

// GetOrStoreRequest receives a tcpStream ident, a timestamp, a request, and a packet count.
//
// If the response that matches the stream ident has been seen before,
// returns a match entry containing both Request and Response and matchFound will be true.
//
// If the response hasn't been seen yet,
// stores the Request for later lookup and returns match as nil and matchFound will be false.
func (m *httpMatcher) GetOrStoreRequest(key int64, timestamp time.Time, request *http.Request, packetCount int) (match *entry, matchFound bool) {
	m.mtx.Lock()
	defer m.mtx.Unlock()

	if e, ok := m.messages[key]; ok {
		e.request = request
		e.requestTimestamp = timestamp
		delete(m.messages, key)
		return e, true
	}

	e := &entry{
		request:            request,
		requestTimestamp:   timestamp,
		requestPacketCount: packetCount,
	}

	m.messages[key] = e
	return nil, false
}

// GetOrStoreResponse receives a tcpStream ident, a timestamp, a response, and a packet count.
//
// If the request that matches the stream ident has been seen before,
// returns a match entry containing both Request and Response and matchFound will be true.
//
// If the request hasn't been seen yet,
// stores the Response for later lookup and returns match as nil and matchFound will be false.
func (m *httpMatcher) GetOrStoreResponse(key int64, timestamp time.Time, response *http.Response, packetCount int) (match *entry, matchFound bool) {
	m.mtx.Lock()
	defer m.mtx.Unlock()

	if e, ok := m.messages[key]; ok {
		e.response = response
		e.responseTimestamp = timestamp
		delete(m.messages, key)
		return e, true
	}

	e := &entry{
		response:            response,
		responseTimestamp:   timestamp,
		responsePacketCount: packetCount,
	}

	m.messages[key] = e
	return nil, false

}
