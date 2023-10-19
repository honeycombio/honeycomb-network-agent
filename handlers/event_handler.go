package handlers

import (
	"context"
	"sync"

	"github.com/honeycombio/honeycomb-network-agent/assemblers"
	"github.com/honeycombio/honeycomb-network-agent/config"
	"github.com/honeycombio/honeycomb-network-agent/utils"
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
	}
	return eventHandler
}
