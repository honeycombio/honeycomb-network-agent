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
	request  *http.Request
	requestTimestamp time.Time

	response *http.Response
	responseTimestamp time.Time
}

func newRequestResponseMatcher() httpMatcher {
	return httpMatcher{
		messages: &sync.Map{},
	}
}

func (m *httpMatcher) LoadOrStoreRequest(requestID string, request *http.Request) *entry {

	// check if we already have a response for this request, if yes, return it
	if e, ok := m.messages.LoadAndDelete(requestID); ok {
		e.(*entry).request = request
		e.(*entry).requestTimestamp = time.Now()
		return e.(*entry)
	}

	// we don't have a response for this request, so store it for later
	entry := entry{
		request:  request,
		requestTimestamp: time.Now(),
	}
	m.messages.Store(requestID, &entry)
	return nil
}

func (m *httpMatcher) LoadOrStoreResponse(requestID string, response *http.Response) *entry {


	// check if we already have a request for this response, if yes, return it
	if e, ok := m.messages.LoadAndDelete(requestID); ok {
		e.(*entry).response = response
		e.(*entry).responseTimestamp = time.Now()
		return e.(*entry)
	}

	// we don't have a request for this response, so store it for later
	entry := entry{
		response: response,
		responseTimestamp: time.Now(),
	}
	m.messages.Store(requestID, &entry)
	return nil
}
