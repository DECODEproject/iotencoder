package clock

import "time"

// Clock is our interface for a type that can be used to tell the time.
// Currently we just expose a Now method as this is all we need, but this could
// be extended.
type Clock interface {
	// Now returns the current time
	Now() time.Time
}

// New returns a new real Clock instance.
func New() Clock {
	return &realClock{}
}

type realClock struct{}

// Now is our implementation of the Now method of the Clock interface. Returns
// the result of time.Now so respects the same monotonicity rules as the real
// time implementation.
func (r *realClock) Now() time.Time {
	return time.Now()
}
