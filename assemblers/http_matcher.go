package assemblers

import (
	"net/http"
	"sync"
	"time"
)

type httpMatcher struct {
	messages map[int64]entry
	sync.Mutex
}

type entry struct {
	request           *http.Request
	requestTimestamp  time.Time
	response          *http.Response
	responseTimestamp time.Time
}

func newRequestResponseMatcher() *httpMatcher {
	return &httpMatcher{
		messages: make(map[int64]entry),
	}
}

// GetOrStoreRequest returns the request for the given key if it exists, otherwise it stores the request and returns nil
// key is the TCP ACK number of the request
func (m *httpMatcher) GetOrStoreRequest(key int64, timestamp time.Time, request *http.Request) (*entry, bool) {
	m.Lock()
	defer m.Unlock()

	// check if we already have a response for this request, if yes, return it
	if e, ok := m.messages[key]; ok {
		e.request = request
		e.requestTimestamp = timestamp
		delete(m.messages, key)
		return &e, true
	}

	// we don't have a response for this request yet, so store it for later
	entry := entry{
		request:          request,
		requestTimestamp: timestamp,
	}
	m.messages[key] = entry
	return nil, false
}

// GetOrStoreResponse returns the response for the given key if it exists, otherwise it stores the response and returns nil
// key is the TCP SEQ number of the response
func (m *httpMatcher) GetOrStoreResponse(key int64, timestamp time.Time, response *http.Response) (*entry, bool) {
	m.Lock()
	defer m.Unlock()

	// check if we already have a request for this response, if yes, return it
	if e, ok := m.messages[key]; ok {
		e.response = response
		e.responseTimestamp = timestamp
		delete(m.messages, key)
		return &e, true
	}

	// we don't have a request for this response yet, so store it for later
	entry := entry{
		response:          response,
		responseTimestamp: timestamp,
	}
	m.messages[key] = entry
	return nil, false
}
