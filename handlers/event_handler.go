package handlers

import (
	"context"

	"github.com/honeycombio/honeycomb-network-agent/assemblers"
)

// EventHandler is an interface for event handlers
type EventHandler interface {
	Start(ctx context.Context)
	handleEvent(event assemblers.HttpEvent)
}
