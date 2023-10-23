package handlers

import (
	"net/http"
	"testing"
	"time"

	"github.com/honeycombio/honeycomb-network-agent/assemblers"
	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/otel/attribute"
)

// test that headerToAttributes correctly sets attributes from headers
func TestHeaderToAttributes(t *testing.T) {
	requestTimestamp := time.Now()
	responseTimestamp := requestTimestamp.Add(3 * time.Millisecond)
	event := createTestOtelEvent(requestTimestamp, responseTimestamp)

	reqAttrs := (headerToAttributes(true, event.Request().Header))

	expectedReqAttrs := []attribute.KeyValue{
		attribute.String("http.request.header.user_agent", "teapot-checker/1.0"),
		attribute.String("http.request.header.connection", "keep-alive"),
	}

	assert.Equal(t, expectedReqAttrs, reqAttrs)

	resAttrs := (headerToAttributes(false, event.Response().Header))
	expectedResAttrs := []attribute.KeyValue{
		attribute.String("http.response.header.content_type", "text/plain; charset=utf-8"),
		attribute.String("http.response.header.x_custom_header", "tea-party"),
	}
	assert.Equal(t, expectedResAttrs, resAttrs)
}

func createTestOtelEvent(requestTimestamp, responseTimestamp time.Time) *assemblers.HttpEvent {
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
			Header:        http.Header{"User-Agent": []string{"teapot-checker/1.0"}, "Connection": []string{"keep-alive"}},
		},
		&http.Response{
			StatusCode:    418,
			ContentLength: 84,
			Header:        http.Header{"Content-Type": []string{"text/plain; charset=utf-8"}, "X-Custom-Header": []string{"tea-party"}},
		},
	)
}
