package mocks

import (
	"sync"

	"github.com/DECODEproject/iotencoder/pkg/mqtt"
)

// MQTTClient is a mock type that implements our mqtt interface. Internally it
// keeps track of subscriptions that it has been asked to create. These can be
// retrieved and checked in tests.
type MQTTClient struct {
	err error

	sync.RWMutex
	Subscriptions map[string]map[string]bool
}

// NewMQTTClient returns a new mock client with the internal map correctly
// initialized.
func NewMQTTClient(err error) *MQTTClient {
	return &MQTTClient{
		err:           err,
		Subscriptions: make(map[string]map[string]bool),
	}
}

// Subscribe is the public interface method. In the mock we add the given broker
// and topic to an internal data structure where it can be retrieved for test
// verification.
func (m *MQTTClient) Subscribe(broker, topic string, cb mqtt.Callback) error {
	if m.err != nil {
		return m.err
	}

	m.Lock()
	defer m.Unlock()

	if _, ok := m.Subscriptions[broker]; !ok {
		m.Subscriptions[broker] = make(map[string]bool)
	}

	m.Subscriptions[broker][topic] = true

	return nil
}

func (m *MQTTClient) Unsubscribe(broker, topic string) error {
	if m.err != nil {
		return m.err
	}

	m.Lock()
	defer m.Unlock()

	if _, ok := m.Subscriptions[broker]; ok {
		if _, ok := m.Subscriptions[broker][topic]; ok {
			delete(m.Subscriptions[broker], topic)
			if len(m.Subscriptions[broker]) == 0 {
				delete(m.Subscriptions, broker)
			}
		}
	}

	return nil
}
