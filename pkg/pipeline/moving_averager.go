package pipeline

import (
	"fmt"
	"sync"
	"time"

	"github.com/DECODEproject/iotencoder/pkg/clock"
	kitlog "github.com/go-kit/kit/log"
)

// MovingAverager is an interface for a type that can return a moving average
// for the given device/sensor/interval
type MovingAverager interface {
	MovingAverage(value float64, deviceToken string, sensorID int, interval uint32) (float64, error)
}

// entry is a type we use to store incoming values which we then calculate a
// moving average for.
type entry struct {
	Timestamp int64
	Value     float64
}

// NewMovingAverager returns an instance of our MovingAverager interface. This
// is a simple in-memory implementation.
func NewMovingAverager(verbose bool, cl clock.Clock, logger kitlog.Logger) MovingAverager {
	return &movingAverager{
		entries: make(map[string][]entry),
		verbose: verbose,
		logger:  logger,
		clock:   cl,
	}
}

// movingAverage is our type that implements the MovingAverager interface using
// a simple in memory store. The store is a map with a key based on the device
// token, sensor id and moving average duration, and values being slices of the
// `entry` type shown above. When a value is received the algorithm is to read
// the slice for the appropriate key, iterate through it removing any entries
// that should no longer be included in the average, and counting and totalling
// the ones that should. The allows us to calculate the average. We then write a
// new slice back into the map.
type movingAverager struct {
	sync.RWMutex
	entries map[string][]entry
	verbose bool
	logger  kitlog.Logger
	clock   clock.Clock
}

// MovingAverage is our implementation of the MovingAverager interface method.
func (m *movingAverager) MovingAverage(value float64, deviceToken string, sensorID int, interval uint32) (float64, error) {
	// build our key for the device/sensor/interval
	key := fmt.Sprintf("%s:%v:%v", deviceToken, sensorID, interval)

	now := m.clock.Now()
	intervalDuration := time.Second * time.Duration(-int(interval))
	previousTime := now.Add(intervalDuration)

	// look in the map for the slice of entries for this combination
	m.RLock()
	entries, ok := m.entries[key]
	m.RUnlock()

	if !ok {
		// no value exists at the moment, so we need to create a new slice, and insert into the map
		entries = []entry{
			entry{
				Timestamp: now.Unix(),
				Value:     value,
			},
		}

		m.Lock()
		m.entries[key] = entries
		m.Unlock()

		// as this is the first value the average value is just the received value
		return value, nil
	}

	newEntries := []entry{}
	counter := 0
	accumulator := 0.0

	// we got some values, so we need to iterate over our entries (including the
	// new value), excluding any that are older than we care about
	for _, e := range entries {
		// if older than our interval we ignore
		if e.Timestamp < previousTime.Unix() {
			continue
		}

		counter = counter + 1
		accumulator = accumulator + e.Value

		newEntries = append(newEntries, e)
	}

	// include the latest entry
	counter = counter + 1
	accumulator = accumulator + value

	newEntries = append(newEntries, entry{
		Timestamp: now.Unix(),
		Value:     value,
	})

	// save to the map
	m.Lock()
	m.entries[key] = newEntries
	m.Unlock()

	return accumulator / float64(counter), nil
}
