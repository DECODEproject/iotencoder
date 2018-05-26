package postgres

import (
	"strings"

	sq "github.com/elgris/sqrl"
	kitlog "github.com/go-kit/kit/log"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"github.com/pkg/errors"
	encoder "github.com/thingful/twirp-encoder-go"
)

// Device is a type used when reading data back from the DB.
type Device struct {
	Broker string `db:"broker"`
	Topic  string `db:"topic"`
}

// Open takes as input a connection string for a DB, and returns either a
// sqlx.DB instance or an error.
func Open(connStr string) (*sqlx.DB, error) {
	return sqlx.Open("postgres", connStr)
}

// DB is our type that wraps an sqlx.DB instance and provides an API for the
// data access functions we require.
type DB struct {
	connStr string
	DB      *sqlx.DB
	logger  kitlog.Logger
}

// NewDB creates a new DB instance with the given connection string. We also
// pass in a logger.
func NewDB(connStr string, logger kitlog.Logger) *DB {
	logger = kitlog.With(logger, "module", "postgres")

	logger.Log("msg", "creating DB instance")

	return &DB{
		connStr: connStr,
		logger:  logger,
	}
}

// Start creates our DB connection pool, and runs all up migrations.
func (d *DB) Start() error {
	d.logger.Log("msg", "starting postgres")

	db, err := Open(d.connStr)
	if err != nil {
		return errors.Wrap(err, "opening db connection failed")
	}

	d.DB = db

	err = MigrateUp(d.DB.DB, d.logger)
	if err != nil {
		return errors.Wrap(err, "running up migrations failed")
	}

	return nil
}

// Stop closes the DB connection pool.
func (d *DB) Stop() error {
	d.logger.Log("msg", "stopping postgres")

	return d.DB.Close()
}

func (d *DB) CreateDevice(req *encoder.CreateStreamRequest) error {
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
		return errors.Wrap(err, "failed to generate device insert sql")
	}

	// rebind for postgres ($1 vs ?)
	sql = d.DB.Rebind(sql)

	_, err = d.DB.Exec(sql, args...)
	if err != nil {
		return errors.Wrap(err, "failed to insert device")
	}

	return nil
}

// GetDevices returns a slice of pointers to Device instances
func (d *DB) GetDevices() ([]*Device, error) {
	sql, args, err := sq.Select("broker", "topic").
		From("devices").ToSql()

	if err != nil {
		return nil, errors.Wrap(err, "failed to generate device select sql")
	}

	sql = d.DB.Rebind(sql)

	devices := []*Device{}

	rows, err := d.DB.Queryx(sql, args...)
	if err != nil {
		return nil, errors.Wrap(err, "failed to select device rows from database")
	}

	for rows.Next() {
		var d Device
		err = rows.StructScan(&d)
		if err != nil {
			return nil, errors.Wrap(err, "failed to scan row into Device struct")
		}

		devices = append(devices, &d)
	}

	return devices, nil
}
