package handlers

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/honeycombio/honeycomb-network-agent/assemblers"
	"github.com/honeycombio/honeycomb-network-agent/config"
	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

func Test_extractContextFromEvent(t *testing.T) {
	// create otel handler
	handler := NewOtelHandler(
		config.Config{
			Endpoint: "https://api.example.com",
		},
		nil,
		nil,
		"").(*otelHandler)
	defer handler.Close()

	traceID, _ := trace.TraceIDFromHex("0af7651916cd43dd8448eb211c80319c")
	spanID, _ := trace.SpanIDFromHex("b7ad6b7169203331")

	// create a test http event
	now := time.Now()
	event := createTestHttpEventWithRequestHeader(now, now, &http.Header{
		"Traceparent": []string{fmt.Sprintf("00-%s-%s-01", traceID, spanID)},
	})

	// use handler to get context from the HTTP event
	ctx := handler.getContextFromHTTPEvent(event)
	// get the span context out of the full event context
	spanCtx := trace.SpanContextFromContext(ctx)

	assert.Equal(t, traceID, spanCtx.TraceID())
	assert.Equal(t, spanID, spanCtx.SpanID())
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

func TestResolveHTTPAttributes(t *testing.T) {
	defaultHandler := NewOtelHandler(
		config.Config{},
		nil,
		nil,
		"").(*otelHandler)
	defer defaultHandler.Close()

	t.Run("no request or response", func(t *testing.T) {
		// no request or response
		emptyEvent := &assemblers.HttpEvent{}
		emptyAttrs := defaultHandler.resolveHTTPAttributes(emptyEvent)

		assert.Contains(t, emptyAttrs, attribute.String("http.request.missing", "no request on this event"))
		assert.Contains(t, emptyAttrs, attribute.String("http.response.missing", "no response on this event"))
	})

	// a "real" HTTP realishEvent
	realishEvent := createTestHttpEvent(time.Now(), time.Now().Add(3*time.Millisecond))

	t.Run("a realish request", func(t *testing.T) {
		attrs := defaultHandler.resolveHTTPAttributes(realishEvent)

		assert.Contains(t, attrs, attribute.String("http.request.method", "GET"))
		assert.Contains(t, attrs, attribute.String("http.method", "GET"))
		assert.Contains(t, attrs, attribute.String("http.request.header.user_agent", "teapot-checker/1.0"))
		assert.Contains(t, attrs, attribute.String("http.request.header.connection", "keep-alive"))
		assert.Contains(t, attrs, attribute.Int("http.response.status_code", 418))
		assert.Contains(t, attrs, attribute.Int("http.status_code", 418))
		assert.Contains(t, attrs, attribute.String("error", "HTTP client error"))
		assert.Contains(t, attrs, attribute.String("http.response.header.content_type", "text/plain; charset=utf-8"))
		assert.Contains(t, attrs, attribute.String("http.response.header.x_custom_header", "tea-party"))
	})

	t.Run("a realish request including URL", func(t *testing.T) {
		moreDetailHandler := NewOtelHandler(
			config.Config{
				IncludeRequestURL: true,
			},
			nil,
			nil,
			"").(*otelHandler)
		defer defaultHandler.Close()

		attrs := moreDetailHandler.resolveHTTPAttributes(realishEvent)

		assert.Contains(t, attrs, attribute.String("url.path", "/check"))
		assert.Contains(t, attrs, attribute.String("http.target", "/check"))
	})
}
