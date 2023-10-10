package handlers

import (
	"context"
	"sync"

	"github.com/honeycombio/honeycomb-network-agent/assemblers"
)

// EventHandler is an interface for event handlers
type EventHandler interface {
	Start(ctx context.Context, wg *sync.WaitGroup)
	Close()
	handleEvent(event assemblers.Event)
}
