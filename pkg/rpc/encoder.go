package rpc

import (
	"context"
	"strings"

	kitlog "github.com/go-kit/kit/log"
	"github.com/pkg/errors"
	encoder "github.com/thingful/twirp-encoder-go"
	"github.com/twitchtv/twirp"

	"github.com/thingful/iotencoder/pkg/mqtt"
	"github.com/thingful/iotencoder/pkg/postgres"
)

// Encoder is our implementation of the generated twirp interface for the
// stream encoder.
type Encoder struct {
	logger kitlog.Logger
	db     postgres.DB
	mqtt   mqtt.Client
}

// ensure we adhere to the interface
var _ encoder.Encoder = &Encoder{}

// NewEncoder returns a newly instantiated Encoder instance. It takes as
// parameters a DB connection string and a logger. The connection string is
// passed down to the postgres package where it is used to connect.
func NewEncoder(logger kitlog.Logger, mqttClient mqtt.Client, db postgres.DB) *Encoder {
	logger = kitlog.With(logger, "module", "rpc")

	logger.Log("msg", "creating encoder")

	return &Encoder{
		logger: logger,
		db:     db,
		mqtt:   mqttClient,
	}
}

// Start the encoder. Here we create MQTT subscriptions for all records stored
// in the DB.
func (e *Encoder) Start() error {
	e.logger.Log("msg", "starting encoder")

	e.logger.Log("msg", "creating existing subscriptions")

	devices, err := e.db.GetDevices()
	if err != nil {
		return errors.Wrap(err, "failed to load devices")
	}

	for _, d := range devices {
		e.logger.Log("broker", d.Broker, "topic", d.Topic, "msg", "creating subscription")

		err = e.mqtt.Subscribe(d.Broker, d.Topic)

		if err != nil {
			return errors.Wrap(err, "failed to subscribe to topic")
		}
	}

	return nil
}

// Stop stops the encoder. Currently this is a NOOP, but keeping the function
// for now.
func (e *Encoder) Stop() error {
	e.logger.Log("msg", "stopping encoder")

	return nil
}

func (e *Encoder) CreateStream(ctx context.Context, req *encoder.CreateStreamRequest) (*encoder.CreateStreamResponse, error) {
	err := validateCreateRequest(req)
	if err != nil {
		return nil, err
	}

	stream := createStream(req)

	streamID, err := e.db.CreateStream(stream)
	if err != nil {
		return nil, twirp.InternalErrorWith(err)
	}

	err = e.mqtt.Subscribe(req.BrokerAddress, req.DeviceTopic)

	if err != nil {
		return nil, twirp.InternalErrorWith(err)
	}

	return &encoder.CreateStreamResponse{
		StreamUid: streamID,
	}, nil
}

func (e *Encoder) DeleteStream(ctx context.Context, req *encoder.DeleteStreamRequest) (*encoder.DeleteStreamResponse, error) {
	return nil, nil
}

// validateCreateRequest is a slightly verbose method that takes as input an
// incoming CreateStreamRequest, and returns a twirp error should any required
// fields are missing, or nil if the request is valid.
func validateCreateRequest(req *encoder.CreateStreamRequest) error {
	if req.BrokerAddress == "" {
		return twirp.RequiredArgumentError("broker_address")
	}

	if req.DeviceTopic == "" {
		return twirp.RequiredArgumentError("device_topic")
	}

	if req.DevicePrivateKey == "" {
		return twirp.RequiredArgumentError("device_private_key")
	}

	if req.RecipientPublicKey == "" {
		return twirp.RequiredArgumentError("recipient_public_key")
	}

	if req.UserUid == "" {
		return twirp.RequiredArgumentError("user_uid")
	}

	if req.Location == nil {
		return twirp.RequiredArgumentError("location")
	}

	if req.Location.Longitude == 0 {
		return twirp.RequiredArgumentError("longitude")
	}

	return nil
}

// createStream is a simple helper method that converts the incoming
// CreateStreamRequest object into a *postgres.Stream instance ready to be
// persisted to the DB.
func createStream(req *encoder.CreateStreamRequest) *postgres.Stream {
	return &postgres.Stream{
		PublicKey: req.RecipientPublicKey,
		Device: &postgres.Device{
			Broker:      req.BrokerAddress,
			Topic:       req.DeviceTopic,
			PrivateKey:  req.DevicePrivateKey,
			UserUID:     req.UserUid,
			Longitude:   req.Location.Longitude,
			Latitude:    req.Location.Latitude,
			Disposition: strings.ToLower(req.Disposition.String()),
		},
	}
}
