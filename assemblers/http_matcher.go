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

func newRequestResponseMatcher() httpMatcher {
	return httpMatcher{
		messages: &sync.Map{},
	}
}

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
