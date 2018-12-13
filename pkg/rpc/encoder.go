package rpc

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	raven "github.com/getsentry/raven-go"
	kitlog "github.com/go-kit/kit/log"
	"github.com/pkg/errors"
	encoder "github.com/thingful/twirp-encoder-go"
	"github.com/twitchtv/twirp"

	"github.com/DECODEproject/iotencoder/pkg/mqtt"
	"github.com/DECODEproject/iotencoder/pkg/postgres"
)

// Processor is the interface we want to call to process incoming events. We
// define it in this package where we need it.
type Processor interface {
	Process(device *postgres.Device, payload []byte) error
}

// encoderImpl is our implementation of the generated twirp interface for the
// stream encoder.
type encoderImpl struct {
	logger       kitlog.Logger
	db           *postgres.DB
	mqtt         mqtt.Client
	brokerAddr   string
	processor    Processor
	verbose      bool
	topicPattern *regexp.Regexp
}

// Config is a struct used to pass in configuration when creating the encoder
type Config struct {
	DB         *postgres.DB
	MQTTClient mqtt.Client
	Processor  Processor
	Verbose    bool
	BrokerAddr string
}

// NewEncoder returns a newly instantiated Encoder instance. It takes as
// parameters a DB connection string and a logger. The connection string is
// passed down to the postgres package where it is used to connect.
func NewEncoder(config *Config, logger kitlog.Logger) encoder.Encoder {
	logger = kitlog.With(logger, "module", "rpc")

	logger.Log("msg", "creating encoder")

	return &encoderImpl{
		logger:       logger,
		db:           config.DB,
		mqtt:         config.MQTTClient,
		processor:    config.Processor,
		verbose:      config.Verbose,
		brokerAddr:   config.BrokerAddr,
		topicPattern: regexp.MustCompile("device/sck/(\\w+)/readings"),
	}
}

// Start the encoder. Here we create MQTT subscriptions for all records stored
// in the DB.
func (e *encoderImpl) Start() error {
	e.logger.Log("msg", "starting encoder")

	e.logger.Log("msg", "creating existing subscriptions")

	devices, err := e.db.GetDevices()
	if err != nil {
		return errors.Wrap(err, "failed to load devices")
	}

	for _, d := range devices {
		e.logger.Log("broker", d.Broker,
			"device_token", d.DeviceToken,
			"msg", "creating subscription",
		)

		err = e.mqtt.Subscribe(
			e.brokerAddr,
			d.DeviceToken,
			func(topic string, payload []byte) {
				e.handleCallback(topic, payload)
			})

		if err != nil {
			e.logger.Log("err", err, "msg", "failed to subscribe to topic")
		}
	}

	return nil
}

// Stop stops the encoder. Currently this is a NOOP, but keeping the function
// for now.
func (e *encoderImpl) Stop() error {
	e.logger.Log("msg", "stopping encoder")

	return nil
}

// CreateStream is our implementation of the protocol buffer interface. It takes
// the incoming request, validates it and if valid we write some data to the
// database, and set up a subscription with the specified MQTT broker.
func (e *encoderImpl) CreateStream(ctx context.Context, req *encoder.CreateStreamRequest) (*encoder.CreateStreamResponse, error) {
	err := validateCreateRequest(req)
	if err != nil {
		raven.CaptureError(err, map[string]string{"operation": "createStream"})
		return nil, err
	}

	stream, err := createStream(req, e.brokerAddr)
	if err != nil {
		raven.CaptureError(err, map[string]string{"operation": "createStream"})
		return nil, err
	}

	stream, err = e.db.CreateStream(stream)
	if err != nil {
		raven.CaptureError(err, map[string]string{"operation": "createStream"})
		return nil, twirp.InternalErrorWith(err)
	}

	err = e.mqtt.Subscribe(e.brokerAddr, req.DeviceToken, func(topic string, payload []byte) {
		e.handleCallback(topic, payload)
	})

	if err != nil {
		raven.CaptureError(err, map[string]string{"operation": "createStream"})
		return nil, twirp.InternalErrorWith(err)
	}

	return &encoder.CreateStreamResponse{
		StreamUid: stream.StreamID,
		Token:     stream.Token,
	}, nil
}

// DeleteStream is the method we provide for deleting a stream. It validates the
// request, then deletes specified records from the database, and removes any
// subscriptions.
func (e *encoderImpl) DeleteStream(ctx context.Context, req *encoder.DeleteStreamRequest) (*encoder.DeleteStreamResponse, error) {
	err := validateDeleteRequest(req)
	if err != nil {
		raven.CaptureError(err, map[string]string{"operation": "deleteStream"})
		return nil, err
	}

	stream := &postgres.Stream{
		StreamID: req.StreamUid,
		Token:    req.Token,
	}

	device, err := e.db.DeleteStream(stream)
	if err != nil {
		raven.CaptureError(err, map[string]string{"operation": "deleteStream"})
		return nil, twirp.InternalErrorWith(err)
	}

	if device != nil {
		// we should unsubscribe for this device
		err = e.mqtt.Unsubscribe(e.brokerAddr, device.DeviceToken)
		if err != nil {
			raven.CaptureError(err, map[string]string{"operation": "deleteStream"})
			return nil, twirp.InternalErrorWith(err)
		}
	}

	return &encoder.DeleteStreamResponse{}, nil
}

// handleCallback is our internal function that receives incoming data from the
// MQTT client. It loads the correct device from Postgres and then dispatches
// processing to the pipeline module which is responsible for manipulating the
// data and then writing to the datastore.
func (e *encoderImpl) handleCallback(topic string, payload []byte) {
	token, err := e.extractToken(topic)
	if err != nil {
		e.logger.Log("err", err, "msg", "failed to extract device token")
	}

	device, err := e.db.GetDevice(token)
	if err != nil {
		e.logger.Log("err", err, "msg", "failed to get device")
	}

	if e.verbose {
		e.logger.Log("topic", topic, "payload", string(payload), "msg", "received data")
	}

	err = e.processor.Process(device, payload)
	if err != nil {
		e.logger.Log("err", err, "msg", "failed to process payload")
	}
}

// validateCreateRequest is a slightly verbose method that takes as input an
// incoming CreateStreamRequest, and returns a twirp error should any required
// fields are missing, or nil if the request is valid.
func validateCreateRequest(req *encoder.CreateStreamRequest) error {
	if req.DeviceToken == "" {
		return twirp.RequiredArgumentError("device_token")
	}

	if req.PolicyId == "" {
		return twirp.RequiredArgumentError("policy_id")
	}

	if req.RecipientPublicKey == "" {
		return twirp.RequiredArgumentError("recipient_public_key")
	}

	if req.Location == nil {
		return twirp.RequiredArgumentError("location")
	}

	if req.Location.Longitude == 0 {
		return twirp.RequiredArgumentError("longitude")
	}

	if req.Location.Longitude < -180 || req.Location.Longitude > 180 {
		return twirp.InvalidArgumentError("longitude", "must be between -180 and 180")
	}

	if req.Location.Latitude == 0 {
		return twirp.RequiredArgumentError("latitude")
	}

	if req.Location.Latitude < -90 || req.Location.Latitude > 90 {
		return twirp.InvalidArgumentError("latitude", "must be between -90 and 90")
	}

	return nil
}

// createStream is a simple helper method that converts the incoming
// CreateStreamRequest object into a *postgres.Stream instance ready to be
// persisted to the DB.
func createStream(req *encoder.CreateStreamRequest, brokerAddr string) (*postgres.Stream, error) {
	operations := []*postgres.Operation{}

	for _, o := range req.Operations {
		operation, err := createOperation(o)
		if err != nil {
			return nil, err
		}

		operations = append(operations, operation)
	}

	return &postgres.Stream{
		PolicyID:   req.PolicyId,
		PublicKey:  req.RecipientPublicKey,
		Operations: operations,
		Device: &postgres.Device{
			Broker:      brokerAddr,
			DeviceToken: req.DeviceToken,
			Longitude:   req.Location.Longitude,
			Latitude:    req.Location.Latitude,
			Exposure:    strings.ToLower(req.Exposure.String()),
		},
	}, nil
}

func createOperation(op *encoder.CreateStreamRequest_Operation) (*postgres.Operation, error) {
	if op.SensorId == 0 {
		return nil, twirp.InvalidArgumentError("operations", "require a non-zero sensor id")
	}

	switch op.Action {
	case encoder.CreateStreamRequest_Operation_SHARE:
		return &postgres.Operation{
			SensorID: op.SensorId,
			Action:   postgres.Action(op.Action.String()),
		}, nil
	case encoder.CreateStreamRequest_Operation_BIN:
		if len(op.Bins) == 0 {
			return nil, twirp.InvalidArgumentError("operations", "binning requires a non-empty list of bins")
		}
		return &postgres.Operation{
			SensorID: op.SensorId,
			Action:   postgres.Action(op.Action.String()),
			Bins:     op.Bins,
		}, nil
	case encoder.CreateStreamRequest_Operation_MOVING_AVG:
		if op.Interval == 0 {
			return nil, twirp.InvalidArgumentError("operations", "moving average requires a non-zero interval")
		}
		return &postgres.Operation{
			SensorID: op.SensorId,
			Action:   postgres.Action(op.Action.String()),
			Interval: op.Interval,
		}, nil
	default:
		return nil, twirp.InvalidArgumentError("operations", "foo")
	}
}

// validateDeleteRequest validates incoming deletion requests (we just check for
// a stream uid)
func validateDeleteRequest(req *encoder.DeleteStreamRequest) error {
	if req.StreamUid == "" {
		return twirp.RequiredArgumentError("stream_uid")
	}

	if req.Token == "" {
		return twirp.RequiredArgumentError("token")
	}

	return nil
}

func (e *encoderImpl) extractToken(topic string) (string, error) {
	matches := e.topicPattern.FindStringSubmatch(topic)

	if matches == nil {
		return "", fmt.Errorf("Unable to extract device token from: %s", topic)
	}

	return matches[1], nil
}
