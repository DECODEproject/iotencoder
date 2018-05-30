package mqtt

import (
	"context"
	"fmt"
	"sync"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	kitlog "github.com/go-kit/kit/log"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	datastore "github.com/thingful/twirp-datastore-go"

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

// Client is the main interface for our MQTT module. It exposes a single method
// Subscribe which attempts to subscribe to the given topic on the specified
// broker, and as events are received it feeds them to a processing pipeline
// which ultimately will end with data being written to the datastore.
type Client interface {
	// Subscribe takes a broker and a topic, and after this function is called the
	// client will have set up a subscription for the given details with received
	// events being written to the datastore. Returns an error if we were unable to
	// subscribe for any reason.
	Subscribe(broker, topic string) error
}

// client abstracts our connection to one or more MQTT brokers, it allows new
// subscriptions to be made to topics, and somehow emits received events to be
// written on to the datastore.
type client struct {
	logger    kitlog.Logger
	db        postgres.DB
	datastore datastore.Datastore

	sync.RWMutex
	clients map[string]mqtt.Client
}

// NewClient creates a new client that is intended to support connections to
// multiple brokers if required. Takes as input
func NewClient(logger kitlog.Logger, db postgres.DB, ds datastore.Datastore) Client {
	logger = kitlog.With(logger, "module", "mqtt")

	logger.Log("msg", "creating mqtt client instance")

	return &client{
		logger:    logger,
		db:        db,
		datastore: ds,
		clients:   make(map[string]mqtt.Client),
	}
}

// Start "starts" the mqtt client. Now this has become a noop, as we lazily
// connect to a broker only on `Subscribe`, so this may be removed.
func (c *client) Start() error {
	c.logger.Log("msg", "starting mqtt client")

	return nil
}

// Stop disconnects all currently connected clients, and clears the map of
// clients
func (c *client) Stop() error {
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
func (c *client) Subscribe(broker, topic string) error {
	c.logger.Log("topic", topic, "broker", broker, "msg", "subscribing")

	var handler mqtt.MessageHandler = func(client mqtt.Client, message mqtt.Message) {
		messageCounter.Inc()

		fmt.Printf("Topic: %s\n", message.Topic())
		fmt.Printf("Message: %s\n", message.Payload())

		device, err := c.db.GetDevice(message.Topic())
		if err != nil {
			c.logger.Log("err", err, "msg", "failed to get device")
			return
		}

		// write all streams to the datastore
		for _, stream := range device.Streams {
			_, err = c.datastore.WriteData(context.Background(), &datastore.WriteRequest{
				PublicKey: stream.PublicKey,
				UserUid:   device.UserUID,
				Data:      message.Payload(),
			})

			if err != nil {
				c.logger.Log("err", err, "msg", "failed to write to datastore")
			}
		}
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
