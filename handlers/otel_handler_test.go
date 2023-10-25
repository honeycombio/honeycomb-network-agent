package handlers

import (
	"net/http"
	"testing"
	"time"

	"github.com/honeycombio/honeycomb-network-agent/config"
	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

func Test_extractContextFromEvent(t *testing.T) {
	// create otel handler
	handler := NewOtelHandler(
		config.Config{
			Endpoint:               "https://api.example.com",
			EnableOtelTraceLinking: true,
		},
		nil,
		nil,
		"").(*otelHandler)
	defer handler.Close()

	// create a test http event
	now := time.Now()
	event := createTestHttpEventWithRequestHeader(now, now, &http.Header{
		"Traceparent": []string{"00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01"},
	})

	// extract the context from the event and ensure it matches the expected values
	ctx := handler.getContextFromEvent(event)
	spanCtx := trace.SpanContextFromContext(ctx)
	traceID, _ := trace.TraceIDFromHex("0af7651916cd43dd8448eb211c80319c")
	assert.Equal(t, spanCtx.TraceID(), traceID)
	spanID, _ := trace.SpanIDFromHex("b7ad6b7169203331")
	assert.Equal(t, spanCtx.SpanID(), spanID)
}

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
