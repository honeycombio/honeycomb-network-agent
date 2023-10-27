package handlers

import (
	"net/http"
	"time"

	"github.com/honeycombio/honeycomb-network-agent/assemblers"
)

func createTestHttpEvent(requestTimestamp, responseTimestamp time.Time) *assemblers.HttpEvent {
	return createTestHttpEventWithRequestHeader(requestTimestamp, responseTimestamp, nil)
}

func createTestHttpEventWithRequestHeader(requestTimestamp, responseTimestamp time.Time, requestHeader *http.Header) *assemblers.HttpEvent {
	if requestHeader == nil {
		requestHeader = &http.Header{"User-Agent": []string{"teapot-checker/1.0"}, "Connection": []string{"keep-alive"}}
	}
	return assemblers.NewHttpEvent(
		"c->s:1->2",
		0,
		requestTimestamp,
		responseTimestamp,
		2,
		3,
		"1.2.3.4",
		"5.6.7.8",
		&http.Request{
			Method:        "GET",
			RequestURI:    "/check?teapot=true",
			ContentLength: 42,
			Header:        *requestHeader,
		},
		&http.Response{
			StatusCode:    418,
			ContentLength: 84,
			Header:        http.Header{"Content-Type": []string{"text/plain; charset=utf-8"}, "X-Custom-Header": []string{"tea-party"}},
		},
	)
}
