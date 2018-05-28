package rpc

import (
	"context"

	"github.com/twitchtv/twirp"

	kitlog "github.com/go-kit/kit/log"
	"github.com/pkg/errors"
	encoder "github.com/thingful/twirp-encoder-go"

	"github.com/thingful/iotencoder/pkg/mqtt"
	"github.com/thingful/iotencoder/pkg/postgres"
)

// Encoder is our implementation of the generated twirp interface for the
// stream encoder.
type Encoder struct {
	logger kitlog.Logger
	db     *postgres.DB
	mqtt   *mqtt.Client
}

// ensure we adhere to the interface
var _ encoder.Encoder = &Encoder{}

// NewEncoder returns a newly instantiated Encoder instance. It takes as
// parameters a DB connection string and a logger. The connection string is
// passed down to the postgres package where it is used to connect.
func NewEncoder(logger kitlog.Logger, mqttClient *mqtt.Client, db *postgres.DB) *Encoder {
	logger = kitlog.With(logger, "module", "rpc")

	logger.Log("msg", "creating encoder")

	return &Encoder{
		logger: logger,
		db:     db,
		mqtt:   mqttClient,
	}
}

// Start - does this actually need to do anything
func (e *Encoder) Start() error {
	e.logger.Log("msg", "starting encoder")

	e.logger.Log("msg", "creating existing subscriptions")

	devices, err := e.db.GetDevices()
	if err != nil {
		return errors.Wrap(err, "failed to load devices")
	}

	for _, d := range devices {
		e.logger.Log("id", d.ID, "private_key", d.PrivateKey)
		err = e.mqtt.Subscribe(d.Broker, d.Topic)
		if err != nil {
			return errors.Wrap(err, "failed to subscribe to topic")
		}
	}

	return nil
}

// Stop stops all child components.
func (e *Encoder) Stop() error {
	e.logger.Log("msg", "stopping encoder")

	return nil
}

func (e *Encoder) CreateStream(ctx context.Context, req *encoder.CreateStreamRequest) (*encoder.CreateStreamResponse, error) {
	err := validateCreateRequest(req)
	if err != nil {
		return nil, twirp.InternalErrorWith(errors.Wrap(err, "failed to validate request"))
	}

	err = e.db.CreateDevice(req)
	if err != nil {
		return nil, twirp.InternalErrorWith(err)
	}

	err = e.mqtt.Subscribe(req.BrokerAddress, req.DeviceTopic)
	if err != nil {
		return nil, twirp.InternalErrorWith(errors.Wrap(err, "failed to subscribe"))
	}

	return &encoder.CreateStreamResponse{}, nil
}

func (e *Encoder) DeleteStream(ctx context.Context, req *encoder.DeleteStreamRequest) (*encoder.DeleteStreamResponse, error) {
	return nil, nil
}

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
