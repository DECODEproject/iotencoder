package system

import "context"

// Startable is a single method interface for a component that can meaningfully
// be "started"
type Startable interface {
	// Start starts the component, creating any runtime resources (connection
	// pools, clients, etc.)
	Start(ctx context.Context) error
}

// Stoppable is a single method interface for a component that can meaningfully be
// "stopped".
type Stoppable interface {
	// Stop stops the component, cleaning up any open resources.
	Stop() error
}
