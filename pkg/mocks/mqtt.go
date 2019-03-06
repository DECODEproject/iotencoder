package mocks

import (
	"fmt"
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
func (m *MQTTClient) Subscribe(broker, username, deviceToken string, cb mqtt.Callback) error {
	if m.err != nil {
		return m.err
	}

	key := fmt.Sprintf("%s:%s", broker, username)

	m.Lock()
	defer m.Unlock()

	if _, ok := m.Subscriptions[key]; !ok {
		m.Subscriptions[key] = make(map[string]bool)
	}

	m.Subscriptions[key][deviceToken] = true

	return nil
}

func (m *MQTTClient) Unsubscribe(broker, username, deviceToken string) error {
	if m.err != nil {
		return m.err
	}

	key := fmt.Sprintf("%s:%s", broker, username)

	m.Lock()
	defer m.Unlock()

	if _, ok := m.Subscriptions[key]; ok {
		if _, ok := m.Subscriptions[key][deviceToken]; ok {
			delete(m.Subscriptions[key], deviceToken)
			if len(m.Subscriptions[key]) == 0 {
				delete(m.Subscriptions, key)
			}
		}
	}

	return nil
}
