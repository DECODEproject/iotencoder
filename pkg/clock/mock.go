package clock

import (
	"sync"
	"time"
)

// Mock is an interface for a manipulable clock for use in tests where we want
// to mess with the time
type Mock interface {
	Clock

	// Set allows the caller to set the time of the Mock clock instance
	Set(t time.Time)

	// Add changes the clocks time by the passed in duration
	Add(d time.Duration)
}

// NewMock creates a new mock clock initialized to the passed in time
func NewMock(t time.Time) Mock {
	return &mockClock{
		baseTime: t,
	}
}

// mockClock is our manipulable clock instance
type mockClock struct {
	sync.Mutex
	baseTime time.Time
}

func (m *mockClock) Now() time.Time {
	defer m.Unlock()
	m.Lock()

	return m.baseTime
}

func (m *mockClock) Set(t time.Time) {
	defer m.Unlock()
	m.Lock()

	m.baseTime = t
}

func (m *mockClock) Add(d time.Duration) {
	defer m.Unlock()
	m.Lock()

	m.baseTime = m.baseTime.Add(d)
}
