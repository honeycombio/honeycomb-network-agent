package handlers

import (
	"context"
	"fmt"
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
	k8sClient  *utils.CachedK8sClient
	eventsChan chan assemblers.HttpEvent
}

// NewLibhoneyEventHandler creates a new event handler that sends events using libhoney
func NewLibhoneyEventHandler(config config.Config, k8sClient *utils.CachedK8sClient, eventsChan chan assemblers.HttpEvent, version string) EventHandler {
	initLibhoney(config, version)
	return &libhoneyEventHandler{
		k8sClient:  k8sClient,
		eventsChan: eventsChan,
	}
}

// Start starts the event handler and begins handling events from the events channel
// When the context is cancelled, the event handler will stop handling events
func (handler *libhoneyEventHandler) Start(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()

	var event assemblers.HttpEvent

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
	libhoney.Init(libhoney.Config{
		APIKey:  config.APIKey,
		Dataset: config.Dataset,
		APIHost: config.Endpoint,
	})

	// appends libhoney's user-agent (TODO: doesn't work, no useragent right now)
	libhoney.UserAgentAddition = fmt.Sprintf("hny-network-agent/%s", version)

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

	return libhoney.Close
}

// setTimestampsAndDurationIfValid sets time-related fields in the emitted telemetry
// about the request/response cycle.
//
// It only sets timestamps if they are present in the captured event, and only
// computes and includes durations for which there are correct timestamps to based them upon.
func setTimestampsAndDurationIfValid(honeyEvent *libhoney.Event, httpEvent assemblers.HttpEvent) {
	honeyEvent.AddField("meta.httpEvent_handled_at", time.Now())
	switch true {
	case httpEvent.RequestTimestamp.IsZero() && httpEvent.ResponseTimestamp.IsZero():
		// no request or response, which is weird, but let's send what we do know
		honeyEvent.AddField("meta.timestamps_missing", "request, response")
		honeyEvent.Timestamp = time.Now()

	case httpEvent.RequestTimestamp.IsZero():
		// no request
		honeyEvent.AddField("meta.timestamps_missing", "request")
		// but we have a response
		honeyEvent.Timestamp = httpEvent.ResponseTimestamp
		honeyEvent.AddField("http.response.timestamp", httpEvent.ResponseTimestamp)
		honeyEvent.AddField("meta.httpEvent_response_handled_latency_ms", time.Since(httpEvent.ResponseTimestamp).Milliseconds())

	case httpEvent.ResponseTimestamp.IsZero(): // have request, no response
		// no response
		honeyEvent.AddField("meta.timestamps_missing", "response")
		// but we have a request
		honeyEvent.Timestamp = httpEvent.RequestTimestamp
		honeyEvent.AddField("http.request.timestamp", httpEvent.RequestTimestamp)
		honeyEvent.AddField("meta.httpEvent_request_handled_latency_ms", time.Since(httpEvent.RequestTimestamp).Milliseconds())

	default: // the happiest of paths, we have both request and response
		honeyEvent.Timestamp = httpEvent.RequestTimestamp
		honeyEvent.AddField("http.request.timestamp", httpEvent.RequestTimestamp)
		honeyEvent.AddField("http.response.timestamp", httpEvent.ResponseTimestamp)
		honeyEvent.AddField("meta.httpEvent_request_handled_latency_ms", time.Since(httpEvent.RequestTimestamp).Milliseconds())
		honeyEvent.AddField("meta.httpEvent_response_handled_latency_ms", time.Since(httpEvent.ResponseTimestamp).Milliseconds())
		honeyEvent.AddField("duration_ms", httpEvent.ResponseTimestamp.Sub(httpEvent.RequestTimestamp).Milliseconds())
	}
}

// handleEvent transforms a captured httpEvent into a libhoney event and sends it
func (handler *libhoneyEventHandler) handleEvent(event assemblers.HttpEvent) {
	// the telemetry event to send
	var ev *libhoney.Event = libhoney.NewEvent()

	setTimestampsAndDurationIfValid(ev, event)

	ev.AddField("meta.stream.ident", event.StreamIdent)

	ev.AddField(string(semconv.ClientSocketAddressKey), event.SrcIp)
	ev.AddField(string(semconv.ServerSocketAddressKey), event.DstIp)

	var requestURI string

	// request attributes
	if event.Request != nil {
		requestURI = event.Request.RequestURI
		ev.AddField("name", fmt.Sprintf("HTTP %s", event.Request.Method))
		ev.AddField(string(semconv.HTTPRequestMethodKey), event.Request.Method)
		ev.AddField(string(semconv.URLPathKey), requestURI)
		ev.AddField(string(semconv.UserAgentOriginalKey), event.Request.Header.Get("User-Agent"))
		ev.AddField(string(semconv.HTTPRequestBodySizeKey), event.Request.ContentLength)
	} else {
		ev.AddField("name", "HTTP")
		ev.AddField("http.request.missing", "no request on this event")
	}

	// response attributes
	if event.Response != nil {
		ev.AddField(string(semconv.HTTPResponseStatusCodeKey), event.Response.StatusCode)
		// We cannot quite follow the OTel spec for HTTP instrumentation and OK/Error Status.
		// https://github.com/open-telemetry/opentelemetry-specification/blob/v1.25.0/specification/trace/semantic_conventions/http.md#status
		// We don't (yet?) have a way to determine the client-or-server perspective of the event,
		// so we'll set the "error" field to the general category of the error status codes.
		if event.Response.StatusCode >= 500 {
			ev.AddField("error", "HTTP server error")
		} else if event.Response.StatusCode >= 400 {
			ev.AddField("error", "HTTP client error")
		}
		ev.AddField(string(semconv.HTTPResponseBodySizeKey), event.Response.ContentLength)

	} else {
		ev.AddField("http.response.missing", "no response on this event")
	}

	ev.Add(utils.GetK8sAttrsForIp(handler.k8sClient, event.SrcIp, "source"))
	ev.Add(utils.GetK8sAttrsForIp(handler.k8sClient, event.DstIp, "destination"))

	log.Debug().
		Str("stream_ident", event.StreamIdent).
		Int64("request_id", event.RequestId).
		Time("event.timestamp", ev.Timestamp).
		Str("http.url", requestURI).
		Msg("Event sent")
	err := ev.Send()
	if err != nil {
		log.Debug().
			Err(err).
			Msg("error sending event")
	}
}
