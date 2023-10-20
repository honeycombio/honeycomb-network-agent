package handlers

import (
	"net/http"
	"testing"
	"time"

	"github.com/honeycombio/honeycomb-network-agent/config"
	"github.com/stretchr/testify/assert"
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
