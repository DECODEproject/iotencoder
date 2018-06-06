package mqtt

import (
	paho "github.com/eclipse/paho.mqtt.golang"
	kitlog "github.com/go-kit/kit/log"
	"github.com/pkg/errors"
)

// Connector is our interface for a type that instantiates a new mqtt.Client
// instance for the specified broker. This logic is defined in an interface so
// that we can supply a mock implementation that does not actually connect to
// any MQTT brokers.
type Connector interface {
	Connect(broker string, logger kitlog.Logger) (paho.Client, error)
}

// NewConnector returns our instantiated connector object, ready for use.
func NewConnector() Connector {
	return &connector{}
}

// connector is our real implementation of the Connector interface.
type connector struct{}

// Connect is a helper function that creates a new mqtt.Client instance that is
// connected to the specified broker.
func (c *connector) Connect(broker string, logger kitlog.Logger) (paho.Client, error) {
	opts, err := createClientOptions(broker, logger)
	if err != nil {
		return nil, err
	}

	logger.Log("broker", broker, "msg", "creating client")

	client := paho.NewClient(opts)
	if token := client.Connect(); token.Wait() && token.Error() != nil {
		return nil, errors.Wrap(token.Error(), "failed to connect to broker")
	}

	logger.Log("broker", broker, "msg", "mqtt connected")

	return client, nil
}

// createClientOptions initializes a set of ClientOptions for connecting to an
// MQTT broker.
func createClientOptions(broker string, logger kitlog.Logger) (*paho.ClientOptions, error) {
	logger.Log("broker", broker, "msg", "configuring client")

	opts := paho.NewClientOptions()
	opts.AddBroker(broker)
	opts.SetClientID(mqttClientID)
	opts.SetAutoReconnect(true)

	return opts, nil
}
