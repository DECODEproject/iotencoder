package system

// Component is a simple interface defining a component that can be started and
// stopped. This is pulled out into a separate package so we can use in various
// places.
type Component interface {
	// Start a component running, returning an error or nil.
	Start() error

	// Stop a component running, returning an error or nil.
	Stop() error
}
