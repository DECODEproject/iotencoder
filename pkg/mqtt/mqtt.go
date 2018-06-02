package mqtt

import (
	"fmt"
	"sync"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	kitlog "github.com/go-kit/kit/log"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/thingful/iotencoder/pkg/version"
)

var (
	// mqttClientID holds a reference to the application ID we send to a broker
	// when connecting
	mqttClientID = fmt.Sprintf("%sDECODE", version.BinaryName)

	// messageCounter is a prometheus counter vec recording the number of received
	// messages, labelled by topic
	messageCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "messages_received",
			Help: "Count of MQTT messages received",
		},
		[]string{"broker"},
	)
)

func init() {
	prometheus.MustRegister(messageCounter)
}

// Callback is a function we pass in to subscribe to a feed.
type Callback func(topic string, payload []byte)

// Client is the main interface for our MQTT module. It exposes a single method
// Subscribe which attempts to subscribe to the given topic on the specified
// broker, and as events are received it feeds them to a processing pipeline
// which ultimately will end with data being written to the datastore.
type Client interface {
	// Subscribe takes a broker and a topic, and after this function is called the
	// client will have set up a subscription for the given details with received
	// events being written to the datastore. Returns an error if we were unable to
	// subscribe for any reason.
	Subscribe(broker, topic string, callback Callback) error

	// Unsubscribe takes a broker and a topic, and attempts to remove the
	// subscription from the specified broker.
	Unsubscribe(broker, topic string) error
}

// client abstracts our connection to one or more MQTT brokers, it allows new
// subscriptions to be made to topics, and somehow emits received events to be
// written on to the datastore.
type client struct {
	logger kitlog.Logger

	sync.RWMutex
	clients map[string]mqtt.Client
}

// NewClient creates a new client that is intended to support connections to
// multiple brokers if required. Takes as input our logger.
func NewClient(logger kitlog.Logger) Client {
	logger = kitlog.With(logger, "module", "mqtt")

	logger.Log("msg", "creating mqtt client instance")

	return &client{
		logger:  logger,
		clients: make(map[string]mqtt.Client),
	}
}

// Stop disconnects all currently connected clients, and clears the map of
// clients
func (c *client) Stop() error {
	c.logger.Log("msg", "stopping mqtt, disconnecting clients")

	c.Lock()
	defer c.Unlock()

	for broker, client := range c.clients {
		client.Disconnect(500)
		delete(c.clients, broker)
	}

	return nil
}

// Subscribe attempts to create a subscription for the given topic, on the given
// broker. This method will create a new connection to particular broker if one
// does not already exist, but will reuse an existing connection.
func (c *client) Subscribe(broker, topic string, cb Callback) error {
	c.logger.Log("topic", topic, "broker", broker, "msg", "subscribing")

	var handler mqtt.MessageHandler = func(client mqtt.Client, message mqtt.Message) {
		messageCounter.With(prometheus.Labels{"broker": broker}).Inc()

		cb(message.Topic(), message.Payload())
	}

	client, err := c.getClient(broker)
	if err != nil {
		return errors.Wrap(err, "failed to get client")
	}

	if token := client.Subscribe(topic, 0, handler); token.Wait() && token.Error() != nil {
		return token.Error()
	}

	return nil
}

// Unsubscribe attempts to unsubscribe to the given topic published on the
// specified broker. We should only unsubscribe when no streams remain for a
// device. Returns any error that occurs while trying to unsubscribe.
func (c *client) Unsubscribe(broker, topic string) error {
	c.logger.Log("broker", broker, "topic", topic, "msg", "unsubscribing")

	client, err := c.getClient(broker)
	if err != nil {
		return errors.Wrap(err, "failed to get client")
	}

	if token := client.Unsubscribe(topic); token.Wait() && token.Error() != nil {
		return token.Error()
	}

	return nil
}

// connect is a helper function that creates a new mqtt.Client instance that is
// connected to the passed in broker.
func connect(broker string, logger kitlog.Logger) (mqtt.Client, error) {
	opts, err := createClientOptions(broker, logger)
	if err != nil {
		return nil, err
	}

	logger.Log("broker", broker, "msg", "creating client")

	client := mqtt.NewClient(opts)
	if token := client.Connect(); token.Wait() && token.Error() != nil {
		return nil, errors.Wrap(token.Error(), "failed to connect to broker")
	}

	logger.Log("broker", broker, "msg", "mqtt connected")

	return client, nil
}

// createClientOptions initializes a set of ClientOptions for connecting to an
// MQTT broker.
func createClientOptions(broker string, logger kitlog.Logger) (*mqtt.ClientOptions, error) {
	logger.Log("broker", broker, "msg", "configuring client")

	opts := mqtt.NewClientOptions()
	opts.AddBroker(broker)
	opts.SetClientID(mqttClientID)
	opts.SetAutoReconnect(true)

	return opts, nil
}

// getClient attempts to get a valid client for a given broker. We first attempt
// to return a client from the in memory process, but if one does not exist we
// use `connect` in order to make a new connection. Once a connnection is made
// it will be stored in memory for use for other subscriptions.
func (c *client) getClient(broker string) (mqtt.Client, error) {
	var client mqtt.Client
	var err error

	// attempt to get client, note the use of RLock here which takes a read only
	// lock on the map containing clients.
	c.RLock()
	client, ok := c.clients[broker]
	c.RUnlock()

	if !ok {
		client, err = connect(broker, c.logger)
		if err != nil {
			return nil, errors.Wrap(err, "failed to connect to broker")
		}

		c.logger.Log("broker", broker, "msg", "storing client")

		c.Lock()
		c.clients[broker] = client
		c.Unlock()
	}

	return client, nil
}
