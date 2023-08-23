package assemblers

import (
	"net/http"
	"sync"
	"time"
)

type httpMatcher struct {
	messages map[string]entry
	sync.Mutex
}

type entry struct {
	request           *http.Request
	requestTimestamp  time.Time
	response          *http.Response
	responseTimestamp time.Time
}

func newRequestResponseMatcher() httpMatcher {
	return httpMatcher{
		messages: make(map[string]entry),
	}
}

func (m *httpMatcher) GetOrStoreRequest(ident string, timestamp time.Time, request *http.Request) (*entry, bool) {
	m.Lock()
	defer m.Unlock()

	// check if we already have a response for this request, if yes, return it
	if e, ok := m.messages[ident]; ok {
		e.request = request
		e.requestTimestamp = timestamp
		return &e, true
	}

	// we don't have a response for this request yet, so store it for later
	entry := entry{
		request:          request,
		requestTimestamp: timestamp,
	}
	m.messages[ident] = entry
	return nil, false
}

func (m *httpMatcher) GetOrStoreResponse(ident string, timestamp time.Time, response *http.Response) (*entry, bool) {
	m.Lock()
	defer m.Unlock()

	// check if we already have a request for this response, if yes, return it
	if e, ok := m.messages[ident]; ok {
		e.response = response
		e.responseTimestamp = timestamp
		return &e, true
	}

	// we don't have a request for this response yet, so store it for later
	entry := entry{
		response:          response,
		responseTimestamp: timestamp,
	}
	m.messages[ident] = entry
	return nil, false
}
