package handlers

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/honeycombio/honeycomb-network-agent/assemblers"
	"github.com/honeycombio/honeycomb-network-agent/config"
	"github.com/honeycombio/honeycomb-network-agent/utils"
	"github.com/honeycombio/otel-config-go/otelconfig"
	"github.com/rs/zerolog/log"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
	"go.opentelemetry.io/otel/trace"
)

type otelHandler struct {
	config       config.Config
	k8sClient    *utils.CachedK8sClient
	eventsChan   chan assemblers.Event
	tracer       trace.Tracer
	otelShutdown func()
}

var _ EventHandler = (*otelHandler)(nil)

// NewOtelHandler creates a new event handler that sends events using OpenTelemetry
func NewOtelHandler(config config.Config, k8sClient *utils.CachedK8sClient, eventsChan chan assemblers.Event, version string) EventHandler {
	otelShutdown, err := otelconfig.ConfigureOpenTelemetry(
		otelconfig.WithServiceName(config.Dataset),
		otelconfig.WithServiceVersion(version),
		otelconfig.WithExporterEndpoint(config.Endpoint),
		otelconfig.WithHeaders(map[string]string{
			"x-honeycomb-team": config.APIKey,
		}),
		otelconfig.WithResourceAttributes(map[string]string{
			"honeycomb.agent_version":        version,
			"meta.agent.node.ip":             config.AgentNodeIP,
			"meta.agent.node.name":           config.AgentNodeName,
			"meta.agent.serviceaccount.name": config.AgentServiceAccount,
			"meta.agent.pod.ip":              config.AgentPodIP,
			"meta.agent.pod.name":            config.AgentPodName,
			"net.component":                  "proxy", // I'm an interstitial! ᕕ( ᐛ )ᕗ
		}),
		otelconfig.WithMetricsEnabled(false),
	)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to configure OpenTelemetry")
	}

	return &otelHandler{
		config:       config,
		k8sClient:    k8sClient,
		eventsChan:   eventsChan,
		tracer:       otel.Tracer(config.Dataset),
		otelShutdown: otelShutdown,
	}
}

// Start starts the event handler and begins handling events from the events channel
// When the context is cancelled, the event handler will stop handling events
func (handler *otelHandler) Start(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()

	var event assemblers.Event
	for {
		select {
		case <-ctx.Done():
			return
		case event = <-handler.eventsChan:
			handler.handleEvent(event)
		}
	}
}

// Close closes the libhoney client, flushing any pending events.
func (handler *otelHandler) Close() {
	handler.otelShutdown()
}

// handleEvent transforms a captured event into a libhoney event and sends it
func (handler *otelHandler) handleEvent(event assemblers.Event) {
	log.Debug().
		Str("stream_ident", event.StreamIdent()).
		Int64("request_id", event.RequestId()).
		Msg("Event sent")

	// Get event start/end timestamps and attributes
	startTime, endTime, attrs := handler.getEventStartEndTimestamps(event)

	switch event.(type) {
	case *assemblers.HttpEvent:
		handler.createHTTPSpan(event.(*assemblers.HttpEvent), startTime, endTime, attrs)
	default:
		log.Warn().Msg("Unknown event type")
		return
	}
}

func (handler *otelHandler) createHTTPSpan(event *assemblers.HttpEvent, startTime, endTime time.Time, attrs []attribute.KeyValue) {
	var spanName string
	if event.Request() == nil {
		spanName = "HTTP"
	} else {
		spanName = fmt.Sprintf("HTTP %s", event.Request().Method)
	}

	_, span := handler.tracer.Start(
		handler.getContextFromHTTPEvent(event),
		spanName,
		trace.WithTimestamp(startTime),
		trace.WithAttributes(
			attribute.String("meta.stream.ident", event.StreamIdent()),
			attribute.Int64("meta.seqack", event.RequestId()),
			attribute.Int("meta.request.packet_count", event.RequestPacketCount()),
			attribute.Int("meta.response.packet_count", event.ResponsePacketCount()),
			semconv.ClientSocketAddress(event.SrcIp()),
			semconv.ServerSocketAddress(event.DstIp()),
		),
		trace.WithAttributes(attrs...),
	)
	defer span.End(trace.WithTimestamp(endTime))

	// request attributes
	if event.Request() != nil {
		span.SetAttributes(
			semconv.HTTPRequestMethodKey.String(event.Request().Method),
			semconv.HTTPRequestBodySize(int(event.Request().ContentLength)),
		)
		// by this point, we've already extracted headers based on HTTP_HEADERS list
		// so we can safely add the headers to the event
		span.SetAttributes(headerToAttributes(true, event.Request().Header)...)
		if handler.config.IncludeRequestURL {
			url, err := url.ParseRequestURI(event.Request().RequestURI)
			if err == nil {
				span.SetAttributes(
					semconv.URLPath(url.Path),
				)
			}
		}
	} else {
		span.SetAttributes(
			attribute.String("http.request.missing", "no request on this event"),
		)
	}

	// response attributes
	if event.Response() != nil {
		span.SetAttributes(
			semconv.HTTPResponseStatusCode(event.Response().StatusCode),
			semconv.HTTPResponseBodySize(int(event.Response().ContentLength)),
		)
		// by this point, we've already extracted headers based on HTTP_HEADERS list
		// so we can safely add the headers to the event
		span.SetAttributes(headerToAttributes(false, event.Response().Header)...)
		// We cannot quite follow the OTel spec for HTTP instrumentation and OK/Error Status.
		// https://github.com/open-telemetry/opentelemetry-specification/blob/v1.25.0/specification/trace/semantic_conventions/http.md#status
		// We don't (yet?) have a way to determine the client-or-server perspective of the event,
		// so we'll set the "error" field to the general category of the error status codes.
		if event.Response().StatusCode >= 500 {
			span.SetAttributes(
				attribute.String("error", "HTTP server error"),
			)
		} else if event.Response().StatusCode >= 400 {
			span.SetAttributes(
				attribute.String("error", "HTTP client error"),
			)
		}
	} else {
		span.SetAttributes(
			attribute.String("http.response.missing", "no response on this event"),
		)
	}

	// TODO: it would be nicer if the k8s attrs were returned as otel attrs instead of a map
	// so that we don't need to loop over them again here
	// Add k8s attributes for source and destination IPs
	for key, val := range handler.k8sClient.GetK8sAttrsForSourceIP(handler.config.AgentPodIP, event.SrcIp()) {
		span.SetAttributes(attribute.String(key, val))
	}
	for key, val := range handler.k8sClient.GetK8sAttrsForDestinationIP(handler.config.AgentPodIP, event.DstIp()) {
		span.SetAttributes(attribute.String(key, val))
	}
}

// getEventStartEndTimestamps sets time-related fields in the emitted telemetry
// about the request/response cycle.
//
// It only sets timestamps if they are present in the captured event, and only
// computes and includes durations for which there are correct timestamps to based them upon.
func (handler *otelHandler) getEventStartEndTimestamps(event assemblers.Event) (time.Time, time.Time, []attribute.KeyValue) {
	var startTime, endTime time.Time
	attrs := []attribute.KeyValue{
		attribute.String("meta.event_handled_at", time.Now().String()),
	}

	switch {
	case event.RequestTimestamp().IsZero() && event.ResponseTimestamp().IsZero(): // no request or response
		attrs = append(attrs, attribute.String("meta.timestamps_missing", "request, response"))
		startTime = time.Now()
		endTime = startTime

	case event.RequestTimestamp().IsZero(): // have response, no request
		attrs = append(attrs, attribute.String("meta.timestamps_missing", "request"))
		startTime = event.ResponseTimestamp()
		endTime = startTime
		attrs = append(attrs, attribute.String("http.response.timestamp", event.ResponseTimestamp().String()))
		attrs = append(attrs, attribute.Int64("meta.response.capture_to_handle.latency_ms", time.Since(event.ResponseTimestamp()).Milliseconds()))

	case event.ResponseTimestamp().IsZero(): // have request, no response
		attrs = append(attrs, attribute.String("meta.timestamps_missing", "response"))
		startTime = event.RequestTimestamp()
		endTime = startTime
		attrs = append(attrs, attribute.String("http.request.timestamp", event.RequestTimestamp().String()))
		attrs = append(attrs, attribute.Int64("meta.request.capture_to_handle.latency_ms", time.Since(event.RequestTimestamp()).Milliseconds()))

	default: // the happiest of paths, we have both request and response
		startTime = event.RequestTimestamp()
		endTime = event.ResponseTimestamp()
		attrs = append(attrs, attribute.String("http.request.timestamp", event.RequestTimestamp().String()))
		attrs = append(attrs, attribute.String("http.response.timestamp", event.ResponseTimestamp().String()))
		attrs = append(attrs, attribute.Int64("meta.request.capture_to_handle.latency_ms", time.Since(event.RequestTimestamp()).Milliseconds()))
		attrs = append(attrs, attribute.Int64("meta.response.capture_to_handle.latency_ms", time.Since(event.ResponseTimestamp()).Milliseconds()))
		attrs = append(attrs, attribute.Int64("duration_ms", event.ResponseTimestamp().Sub(event.RequestTimestamp()).Milliseconds()))

	}
	return startTime, endTime, attrs
}

// headerToAttributes converts a http.Header into a slice of OpenTelemetry attributes
func headerToAttributes(isRequest bool, header http.Header) []attribute.KeyValue {
	attrs := []attribute.KeyValue{}
	for key, val := range sanitizeHeaders(isRequest, header) {
		attrs = append(attrs, attribute.String(key, val))
	}
	return attrs
}

// getContextFromHTTPEvent attempts to extract OTEL trace context from a HTTP event's request headers
//
// If present, it returns a new context with the extracted trace context.
//
// If not, it returns a new empty context.
func (handler *otelHandler) getContextFromHTTPEvent(event *assemblers.HttpEvent) context.Context {
	ctx := context.Background()
	if event.Request() != nil {
		ctx = otel.GetTextMapPropagator().Extract(ctx, propagation.HeaderCarrier(event.Request().Header))
	}
	return ctx
}
