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
	"github.com/honeycombio/libhoney-go"
	"github.com/rs/zerolog/log"

	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
)

// libhoneyEventHandler is an event handler that sends events using libhoney
type libhoneyEventHandler struct {
	config     config.Config
	k8sClient  *utils.CachedK8sClient
	eventsChan chan assemblers.Event
}

var _ EventHandler = (*libhoneyEventHandler)(nil)

// NewLibhoneyEventHandler creates a new event handler that sends events using libhoney
func NewLibhoneyEventHandler(config config.Config, k8sClient *utils.CachedK8sClient, eventsChan chan assemblers.Event, version string) EventHandler {
	initLibhoney(config, version)
	return &libhoneyEventHandler{
		config:     config,
		k8sClient:  k8sClient,
		eventsChan: eventsChan,
	}
}

// Start starts the event handler and begins handling events from the events channel
// When the context is cancelled, the event handler will stop handling events
func (handler *libhoneyEventHandler) Start(ctx context.Context, wg *sync.WaitGroup) {
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
func (handler *libhoneyEventHandler) Close() {
	libhoney.Close()
}

// initLibhoney initializes libhoney and sets global fields
func initLibhoney(config config.Config, version string) func() {
	// appends libhoney's user-agent, has to happen before libhoney.Init()
	libhoney.UserAgentAddition = fmt.Sprintf("hny-network-agent/%s", version)

	libhoney.Init(libhoney.Config{
		APIKey:  config.APIKey,
		Dataset: config.Dataset,
		APIHost: config.Endpoint,
	})

	// configure global fields that are set on all events
	libhoney.AddField("honeycomb.agent_version", version)

	if config.AgentNodeIP != "" {
		libhoney.AddField("meta.agent.node.ip", config.AgentNodeIP)
	}
	if config.AgentNodeName != "" {
		libhoney.AddField("meta.agent.node.name", config.AgentNodeName)
	}
	if config.AgentServiceAccount != "" {
		libhoney.AddField("meta.agent.serviceaccount.name", config.AgentServiceAccount)
	}
	// because we use hostnetwork in deployments, the pod IP and node IP are the same
	if config.AgentPodIP != "" {
		libhoney.AddField("meta.agent.pod.ip", config.AgentPodIP)
	}
	if config.AgentPodName != "" {
		libhoney.AddField("meta.agent.pod.name", config.AgentPodName)
	}
	for k, v := range config.AdditionalAttributes {
		libhoney.AddField(k, v)
	}

	return libhoney.Close
}

// handleEvent transforms a captured event into a libhoney event and sends it
func (handler *libhoneyEventHandler) handleEvent(event assemblers.Event) {
	// the telemetry event to send
	var ev *libhoney.Event = libhoney.NewEvent()

	handler.setTimestampsAndDurationIfValid(ev, event)

	ev.AddField("meta.stream.ident", event.StreamIdent())
	ev.AddField("meta.seqack", event.RequestId())
	ev.AddField("meta.request.packet_count", event.RequestPacketCount())
	ev.AddField("meta.response.packet_count", event.ResponsePacketCount())

	ev.AddField(string(semconv.ClientSocketAddressKey), event.SrcIp())
	ev.AddField(string(semconv.ServerSocketAddressKey), event.DstIp())

	// add custom fields based on the event type
	switch event.(type) {
	case *assemblers.HttpEvent:
		handler.addHttpFields(ev, event.(*assemblers.HttpEvent))
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
func (handler *libhoneyEventHandler) setTimestampsAndDurationIfValid(honeyEvent *libhoney.Event, event assemblers.Event) {
	honeyEvent.AddField("meta.event_handled_at", time.Now())
	switch {
	case event.RequestTimestamp().IsZero() && event.ResponseTimestamp().IsZero():
		// no request or response, which is weird, but let's send what we do know
		honeyEvent.AddField("meta.timestamps_missing", "request, response")
		honeyEvent.Timestamp = time.Now()

	case event.RequestTimestamp().IsZero():
		// no request
		honeyEvent.AddField("meta.timestamps_missing", "request")
		// but we have a response
		honeyEvent.Timestamp = event.ResponseTimestamp()
		honeyEvent.AddField("http.response.timestamp", event.ResponseTimestamp())
		honeyEvent.AddField("meta.response.capture_to_handle.latency_ms", time.Since(event.ResponseTimestamp()).Milliseconds())

	case event.ResponseTimestamp().IsZero(): // have request, no response
		// no response
		honeyEvent.AddField("meta.timestamps_missing", "response")
		// but we have a request
		honeyEvent.Timestamp = event.RequestTimestamp()
		honeyEvent.AddField("http.request.timestamp", event.RequestTimestamp())
		honeyEvent.AddField("meta.request.capture_to_handle.latency_ms", time.Since(event.RequestTimestamp()).Milliseconds())

	default: // the happiest of paths, we have both request and response
		honeyEvent.Timestamp = event.RequestTimestamp()
		honeyEvent.AddField("http.request.timestamp", event.RequestTimestamp())
		honeyEvent.AddField("http.response.timestamp", event.ResponseTimestamp())
		honeyEvent.AddField("meta.request.capture_to_handle.latency_ms", time.Since(event.RequestTimestamp()).Milliseconds())
		honeyEvent.AddField("meta.response.capture_to_handle.latency_ms", time.Since(event.ResponseTimestamp()).Milliseconds())
		honeyEvent.AddField("duration_ms", event.ResponseTimestamp().Sub(event.RequestTimestamp()).Milliseconds())
	}
}

func (handler *libhoneyEventHandler) addHttpFields(ev *libhoney.Event, event *assemblers.HttpEvent) {
	// request attributes
	if event.Request() != nil {
		ev.AddField("name", fmt.Sprintf("HTTP %s", event.Request().Method))
		ev.AddField(string(semconv.HTTPRequestMethodKey), event.Request().Method)
		ev.AddField(string(semconv.UserAgentOriginalKey), event.Request().Header.Get("User-Agent"))
		ev.AddField(string(semconv.HTTPRequestBodySizeKey), event.Request().ContentLength)
		if handler.config.IncludeRequestURL {
			url, err := url.ParseRequestURI(event.Request().RequestURI)
			if err == nil {
				ev.AddField(string(semconv.URLPathKey), url.Path)
			}
		}
		// by this point, we've already extracted headers based on HTTP_HEADERS list
		// so we can safely add the headers to the event
		ev.AddField("http.request.headers", event.Request().Header)
	} else {
		ev.AddField("name", "HTTP")
		ev.AddField("http.request.missing", "no request on this event")
	}

	// response attributes
	if event.Response() != nil {
		ev.AddField(string(semconv.HTTPResponseStatusCodeKey), event.Response().StatusCode)
		// We cannot quite follow the OTel spec for HTTP instrumentation and OK/Error Status.
		// https://github.com/open-telemetry/opentelemetry-specification/blob/v1.25.0/specification/trace/semantic_conventions/http.md#status
		// We don't (yet?) have a way to determine the client-or-server perspective of the event,
		// so we'll set the "error" field to the general category of the error status codes.
		if event.Response().StatusCode >= 500 {
			ev.AddField("error", "HTTP server error")
		} else if event.Response().StatusCode >= 400 {
			ev.AddField("error", "HTTP client error")
		}
		ev.AddField(string(semconv.HTTPResponseBodySizeKey), event.Response().ContentLength)
		// by this point, we've already extracted headers based on HTTP_HEADERS list
		// so we can safely add the headers to the event
		ev.AddField("http.response.headers", event.Response().Header)
	} else {
		ev.AddField("http.response.missing", "no response on this event")
	}
}
