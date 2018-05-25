package rpc

import (
	"context"
	"strings"

	"github.com/twitchtv/twirp"

	sq "github.com/elgris/sqrl"
	kitlog "github.com/go-kit/kit/log"
	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"
	encoder "github.com/thingful/twirp-encoder-go"

	"github.com/thingful/iotencoder/pkg/postgres"
)

// Encoder is our implementation of the generated twirp interface for the
// stream encoder.
type Encoder struct {
	DB      *sqlx.DB
	connStr string
	logger  kitlog.Logger
}

// ensure we adhere to the interface
var _ encoder.Encoder = &Encoder{}

// NewEncoder returns a newly instantiated Encoder instance. It takes as
// parameters a DB connection string and a logger. The connection string is
// passed down to the postgres package where it is used to connect.
func NewEncoder(connStr string, logger kitlog.Logger) *Encoder {
	logger = kitlog.With(logger, "module", "rpc")

	return &Encoder{
		connStr: connStr,
		logger:  logger,
	}
}

// Start starts all child components (currently just the postgres DB).
func (e *Encoder) Start() error {
	e.logger.Log("msg", "starting encoder")

	db, err := postgres.Open(e.connStr)
	if err != nil {
		return errors.Wrap(err, "opening db connection failed")
	}

	e.DB = db

	err = postgres.MigrateUp(e.DB.DB, e.logger)
	if err != nil {
		return errors.Wrap(err, "running up migrations failed")
	}

	return nil
}

// Stop stops all child components.
func (e *Encoder) Stop() error {
	e.logger.Log("msg", "stopping datastore")

	return e.DB.Close()
}

func (e *Encoder) CreateStream(ctx context.Context, req *encoder.CreateStreamRequest) (*encoder.CreateStreamResponse, error) {
	err := validateCreateRequest(req)
	if err != nil {
		return nil, twirp.InternalErrorWith(errors.Wrap(err, "failed to validate request"))
	}

	sql, args, err := sq.Insert("devices").
		Columns("broker", "topic", "private_key", "user_uid", "longitude", "latitude", "disposition").
		Values(
			req.BrokerAddress,
			req.DeviceTopic,
			req.DevicePrivateKey,
			req.UserUid,
			req.Location.Longitude,
			req.Location.Latitude,
			strings.ToLower(req.Disposition.String()),
		).
		ToSql()

	if err != nil {
		return nil, twirp.InternalErrorWith(errors.Wrap(err, "failed to generate device insert sql"))
	}

	// rebind for postgres ($1 vs ?)
	sql = e.DB.Rebind(sql)

	_, err = e.DB.Exec(sql, args...)
	if err != nil {
		return nil, twirp.InternalErrorWith(errors.Wrap(err, "failed to insert device when creating stream"))
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
