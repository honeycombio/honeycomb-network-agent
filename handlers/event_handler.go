package handlers

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/honeycombio/honeycomb-network-agent/assemblers"
	"github.com/honeycombio/honeycomb-network-agent/config"
	"github.com/honeycombio/honeycomb-network-agent/utils"
	"github.com/rs/zerolog/log"
)

// EventHandler is an interface for event handlers
type EventHandler interface {
	Start(ctx context.Context, wg *sync.WaitGroup)
	Close()
	handleEvent(event assemblers.Event)
}

// NewEventHandler returns an event handler based on the config's selected handler type.
func NewEventHandler(config config.Config, cachedK8sClient *utils.CachedK8sClient, eventsChannel chan assemblers.Event, version string) EventHandler {
	var eventHandler EventHandler
	switch config.EventHandlerType {
	case "libhoney":
		eventHandler = NewLibhoneyEventHandler(config, cachedK8sClient, eventsChannel, version)
	case "otel":
		eventHandler = NewOtelHandler(config, cachedK8sClient, eventsChannel, version)
	default:
		log.Warn().Str("event_handler_type", config.EventHandlerType).Msg("Unknown event handler type. Using libhoney.")
		eventHandler = NewLibhoneyEventHandler(config, cachedK8sClient, eventsChannel, version)
	}
	return eventHandler
}

// santitizeHeaders takes a map of headers and returns a new map with the keys sanitized
// sanitization involves:
// - converting the keys to lowercase
// - replacing - with _
// - prepending http.request.header or http.response.header
func santitizeHeaders(isRequest bool, header http.Header) map[string]string {
	var prefix string
	if isRequest {
		prefix = "http.request.header"
	} else {
		prefix = "http.response.header"
	}

	headers := make(map[string]string, len(header))
	for key, values := range header {
		// OTel semantic conventions suggest lowercase, with - characters replaced by _
		sanitizedKey := strings.ToLower(strings.Replace(key, "-", "_", -1))
		headers[fmt.Sprintf("%s.%s", prefix, sanitizedKey)] = strings.Join(values, ",")
	}
	return headers
}
