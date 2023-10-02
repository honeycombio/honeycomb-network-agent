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

	if config.ClusterName != "" {
		libhoney.AddField("meta.cluster.name", config.ClusterName)
	}
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

func (handler *libhoneyEventHandler) handleEvent(event assemblers.HttpEvent) {
	// create libhoney event
	ev := libhoney.NewEvent()

	// calculate event duration
	// TODO: This is a hack to work around a bug that results in the response timestamp sometimes
	// being zero which causes the event duration to be negative.
	if event.RequestTimestamp.IsZero() {
		log.Debug().
			Str("stream_ident", event.StreamIdent).
			Int64("request_id", event.RequestId).
			Msg("Request timestamp is zero")
		ev.AddField("http.request.timestamp_missing", true)
		event.RequestTimestamp = time.Now()
	}
	if event.ResponseTimestamp.IsZero() {
		log.Debug().
			Str("stream_ident", event.StreamIdent).
			Int64("request_id", event.RequestId).
			Msg("Response timestamp is zero")
		ev.AddField("http.response.timestamp_missing", true)
		event.ResponseTimestamp = time.Now()
	}
	eventDuration := event.ResponseTimestamp.Sub(event.RequestTimestamp)

	// common attributes
	ev.Timestamp = event.RequestTimestamp
	ev.AddField("meta.httpEvent_handled_at", time.Now())
	ev.AddField("meta.httpEvent_request_handled_latency_ms", time.Since(event.RequestTimestamp).Milliseconds())
	ev.AddField("meta.httpEvent_response_handled_latency_ms", time.Since(event.ResponseTimestamp).Milliseconds())
	ev.AddField("meta.stream.ident", event.StreamIdent)
	ev.AddField("duration_ms", eventDuration.Milliseconds())
	ev.AddField("http.request.timestamp", event.RequestTimestamp)
	ev.AddField("http.response.timestamp", event.ResponseTimestamp)

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
