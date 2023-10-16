package handlers

import (
	"context"
	"fmt"
	"net/url"
	"sync"
	"time"

	"github.com/honeycombio/honeycomb-network-agent/assemblers"
	"github.com/honeycombio/honeycomb-network-agent/config"
	"github.com/honeycombio/honeycomb-network-agent/utils"
	"github.com/honeycombio/otel-config-go/otelconfig"
	"github.com/rs/zerolog/log"
	"go.opentelemetry.io/otel"
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
			"x-honeycomb-team":    config.APIKey,
			"x-honeycomb-dataset": config.Dataset,
		}),
		otelconfig.WithResourceAttributes(map[string]string{
			"honeycomb.agent_version": version,
		}),
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
	// the telemetry event to send
	_, span := handler.tracer.
		Start(context.Background(), "HTTP ",
			trace.WithTimestamp(event.RequestTimestamp()),
		)

	handler.setTimestampsAndDurationIfValid(span, event)

	ev.AddField("meta.stream.ident", event.StreamIdent())
	ev.AddField("meta.seqack", event.RequestId())
	ev.AddField("meta.request.packet_count", event.RequestPacketCount())
	ev.AddField("meta.response.packet_count", event.ResponsePacketCount())

	ev.AddField(string(semconv.ClientSocketAddressKey), event.SrcIp())
	ev.AddField(string(semconv.ServerSocketAddressKey), event.DstIp())

	// add custom fields based on the event type
	switch event.(type) {
	case *assemblers.HttpEvent:
		handler.addHttpFields(span, event.(*assemblers.HttpEvent))
	}

	ev.Add(handler.k8sClient.GetK8sAttrsForSourceIP(handler.config.AgentPodIP, event.SrcIp()))
	ev.Add(handler.k8sClient.GetK8sAttrsForDestinationIP(handler.config.AgentPodIP, event.DstIp()))

	log.Debug().
		Str("stream_ident", event.StreamIdent()).
		Int64("request_id", event.RequestId()).
		Time("event.timestamp", ev.Timestamp).
		Msg("Event sent")
	err := ev.Send()
	if err != nil {
		log.Debug().
			Err(err).
			Msg("error sending event")
	}
}

// setTimestampsAndDurationIfValid sets time-related fields in the emitted telemetry
// about the request/response cycle.
//
// It only sets timestamps if they are present in the captured event, and only
// computes and includes durations for which there are correct timestamps to based them upon.
func (handler *otelHandler) setTimestampsAndDurationIfValid(span trace.Span, event assemblers.Event) {
	span.AddField("meta.event_handled_at", time.Now())
	switch {
	case event.RequestTimestamp().IsZero() && event.ResponseTimestamp().IsZero():
		// no request or response, which is weird, but let's send what we do know
		span.AddField("meta.timestamps_missing", "request, response")
		span.Timestamp = time.Now()

	case event.RequestTimestamp().IsZero():
		// no request
		span.AddField("meta.timestamps_missing", "request")
		// but we have a response
		span.Timestamp = event.ResponseTimestamp()
		span.AddField("http.response.timestamp", event.ResponseTimestamp())
		span.AddField("meta.response.capture_to_handle.latency_ms", time.Since(event.ResponseTimestamp()).Milliseconds())

	case event.ResponseTimestamp().IsZero(): // have request, no response
		// no response
		span.AddField("meta.timestamps_missing", "response")
		// but we have a request
		span.Timestamp = event.RequestTimestamp()
		span.AddField("http.request.timestamp", event.RequestTimestamp())
		span.AddField("meta.request.capture_to_handle.latency_ms", time.Since(event.RequestTimestamp()).Milliseconds())

	default: // the happiest of paths, we have both request and response
		span.Timestamp = event.RequestTimestamp()
		span.AddField("http.request.timestamp", event.RequestTimestamp())
		span.AddField("http.response.timestamp", event.ResponseTimestamp())
		span.AddField("meta.request.capture_to_handle.latency_ms", time.Since(event.RequestTimestamp()).Milliseconds())
		span.AddField("meta.response.capture_to_handle.latency_ms", time.Since(event.ResponseTimestamp()).Milliseconds())
		span.AddField("duration_ms", event.ResponseTimestamp().Sub(event.RequestTimestamp()).Milliseconds())
	}
}

func (handler *otelHandler) addHttpFields(span trace.Span, event *assemblers.HttpEvent) {
	// request attributes
	if event.Request() != nil {
		span.AddField("name", fmt.Sprintf("HTTP %s", event.Request().Method))
		span.AddField(string(semconv.HTTPRequestMethodKey), event.Request().Method)
		span.AddField(string(semconv.UserAgentOriginalKey), event.Request().Header.Get("User-Agent"))
		span.AddField(string(semconv.HTTPRequestBodySizeKey), event.Request().ContentLength)
		if handler.config.IncludeRequestURL {
			url, err := url.ParseRequestURI(event.Request().RequestURI)
			if err == nil {
				span.AddField(string(semconv.URLPathKey), url.Path)
			}
		}
	} else {
		span.AddField("name", "HTTP")
		span.AddField("http.request.missing", "no request on this event")
	}

	// response attributes
	if event.Response() != nil {
		span.AddField(string(semconv.HTTPResponseStatusCodeKey), event.Response().StatusCode)
		// We cannot quite follow the OTel spec for HTTP instrumentation and OK/Error Status.
		// https://github.com/open-telemetry/opentelemetry-specification/blob/v1.25.0/specification/trace/semantic_conventions/http.md#status
		// We don't (yet?) have a way to determine the client-or-server perspective of the event,
		// so we'll set the "error" field to the general category of the error status codes.
		if event.Response().StatusCode >= 500 {
			span.AddField("error", "HTTP server error")
		} else if event.Response().StatusCode >= 400 {
			span.AddField("error", "HTTP client error")
		}
		span.AddField(string(semconv.HTTPResponseBodySizeKey), event.Response().ContentLength)
	} else {
		span.AddField("http.response.missing", "no response on this event")
	}
}
