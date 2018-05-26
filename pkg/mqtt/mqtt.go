package mqtt

import (
	"fmt"
	"sync"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	kitlog "github.com/go-kit/kit/log"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/thingful/iotencoder/pkg/postgres"
	"github.com/thingful/iotencoder/pkg/version"
)

var (
	// mqttClientID holds a reference to the application ID we send to a broker
	// when connecting
	mqttClientID = fmt.Sprintf("%s_decode", version.BinaryName)

	// messageCounter is a prometheus counter recording the number of received
	// messages
	messageCounter = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "messages_received",
			Help: "Count of MQTT messages received",
		},
	)
)

func init() {
	prometheus.MustRegister(messageCounter)
}

// Client abstracts our connection to one or more MQTT brokers, it allows new
// subscriptions to be made to topics, and somehow emits received events to be
// written on to the datastore.
type Client struct {
	logger kitlog.Logger

	sync.RWMutex
	clients map[string]mqtt.Client
}

func NewClient(logger kitlog.Logger, db *postgres.DB) *Client {
	logger = kitlog.With(logger, "module", "mqtt")

	logger.Log("msg", "creating mqtt client instance")

	return &Client{
		logger:  logger,
		clients: make(map[string]mqtt.Client),
	}
}

func (c *Client) Start() error {
	c.logger.Log("msg", "starting mqtt client")

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

// Stop disconnects all currently connected clients, and clears the map of
// clients
func (c *Client) Stop() error {
	c.Lock()
	for broker, client := range c.clients {
		client.Disconnect(500)
		delete(c.clients, broker)
	}
	c.Unlock()

	return nil
}

// Subscribe attempts to create a subscription for the given topic, on the given
// broker. This method will create a new connection to particular broker if one
// does not already exist, but will reuse an existing connection.
func (c *Client) Subscribe(broker, topic string) error {
	c.logger.Log("topic", topic, "broker", broker, "msg", "subscribing")

	var msgHandler mqtt.MessageHandler = func(client mqtt.Client, message mqtt.Message) {
		messageCounter.Inc()

		fmt.Printf("Topic: %s\n", message.Topic())
		fmt.Printf("Message: %s\n", message.Payload())
	}

	client, err := c.getClient(broker)
	if err != nil {
		return errors.Wrap(err, "failed to get client")
	}

	if token := client.Subscribe(topic, 0, msgHandler); token.Wait() && token.Error() != nil {
		return token.Error()
	}

	return nil
}

func (c *Client) getClient(broker string) (mqtt.Client, error) {
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

		c.logger.Log("broker", broker, "msg", "adding client to map")

		c.Lock()
		c.clients[broker] = client
		c.Unlock()
	}

	return client, nil
}
