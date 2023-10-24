package handlers

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/otel/attribute"
)

// test that headerToAttributes correctly sets attributes from headers
func TestHeaderToAttributes(t *testing.T) {
	requestTimestamp := time.Now()
	responseTimestamp := requestTimestamp.Add(3 * time.Millisecond)
	event := createTestHttpEvent(requestTimestamp, responseTimestamp)

	reqAttrs := headerToAttributes(true, event.Request().Header)
	assert.Contains(t, reqAttrs, attribute.String("http.request.header.user_agent", "teapot-checker/1.0"))
	assert.Contains(t, reqAttrs, attribute.String("http.request.header.connection", "keep-alive"))

	resAttrs := headerToAttributes(false, event.Response().Header)
	assert.Contains(t, resAttrs, attribute.String("http.response.header.content_type", "text/plain; charset=utf-8"))
	assert.Contains(t, resAttrs, attribute.String("http.response.header.x_custom_header", "tea-party"))
}
