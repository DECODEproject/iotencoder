package rpc

import (
	"context"

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
	return nil, nil
}

func (e *Encoder) DeleteStream(ctx context.Context, req *encoder.DeleteStreamRequest) (*encoder.DeleteStreamResponse, error) {
	return nil, nil
}
