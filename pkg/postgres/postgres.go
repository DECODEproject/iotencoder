package postgres

import (
	"strings"

	kitlog "github.com/go-kit/kit/log"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"github.com/pkg/errors"
	encoder "github.com/thingful/twirp-encoder-go"
)

// Device is a type used when reading data back from the DB.
type Device struct {
	ID         int    `db:"id"`
	Broker     string `db:"broker"`
	Topic      string `db:"topic"`
	PrivateKey string `db:"private_key"`
}

// Open takes as input a connection string for a DB, and returns either a
// sqlx.DB instance or an error.
func Open(connStr string) (*sqlx.DB, error) {
	return sqlx.Open("postgres", connStr)
}

// DB is our type that wraps an sqlx.DB instance and provides an API for the
// data access functions we require.
type DB struct {
	connStr            string
	encryptionPassword []byte
	DB                 *sqlx.DB
	logger             kitlog.Logger
}

// NewDB creates a new DB instance with the given connection string. We also
// pass in a logger.
func NewDB(connStr, encryptionPassword string, logger kitlog.Logger) *DB {
	logger = kitlog.With(logger, "module", "postgres")

	logger.Log("msg", "creating DB instance")

	return &DB{
		connStr:            connStr,
		encryptionPassword: []byte(encryptionPassword),
		logger:             logger,
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

func (d *DB) CreateDevice(req *encoder.CreateStreamRequest) (err error) {
	sql := `INSERT INTO devices
	(broker, topic, private_key, user_uid, longitude, latitude, disposition)
	VALUES (:broker, :topic, pgp_sym_encrypt(:private_key, :encryption_password), :user_uid, :longitude, :latitude, :disposition)
	ON CONFLICT (topic) DO UPDATE
	SET broker = EXCLUDED.broker,
			longitude = EXCLUDED.longitude,
			latitude = EXCLUDED.latitude,
			disposition = EXCLUDED.disposition
	RETURNING id`

	mapArgs := map[string]interface{}{
		"broker":              req.BrokerAddress,
		"topic":               req.DeviceTopic,
		"private_key":         req.DevicePrivateKey,
		"user_uid":            req.UserUid,
		"longitude":           req.Location.Longitude,
		"latitude":            req.Location.Latitude,
		"disposition":         strings.ToLower(req.Disposition.String()),
		"encryption_password": d.encryptionPassword,
	}

	tx, err := d.DB.Beginx()
	if err != nil {
		return errors.Wrap(err, "failed to start transaction when inserting device")
	}

	defer func() {
		if err != nil {
			tx.Rollback()
			return
		}

		err = tx.Commit()
	}()

	sql, args, err := tx.BindNamed(sql, mapArgs)
	if err != nil {
		return errors.Wrap(err, "failed to rebind args")
	}

	var deviceId int

	err = tx.Get(&deviceId, sql, args...)
	if err != nil {
		return errors.Wrap(err, "failed to upsert device")
	}

	sql = `INSERT INTO streams
	(device_id, public_key)
	VALUES (:device_id, pgp_sym_encrypt(:public_key, :encryption_password))`

	mapArgs = map[string]interface{}{
		"device_id":           deviceId,
		"public_key":          req.RecipientPublicKey,
		"encryption_password": d.encryptionPassword,
	}

	sql, args, err = tx.BindNamed(sql, mapArgs)
	if err != nil {
		return errors.Wrap(err, "failed to rebind args")
	}

	_, err = tx.Exec(sql, args...)
	if err != nil {
		return errors.Wrap(err, "failed to insert stream")
	}

	return err
}

// GetDevices returns a slice of pointers to Device instances
func (d *DB) GetDevices() ([]*Device, error) {
	sql := `SELECT id, broker, topic, pgp_sym_decrypt(private_key, :encryption_password) AS private_key
		FROM devices`

	mapArgs := map[string]interface{}{
		"encryption_password": d.encryptionPassword,
	}

	sql, args, err := d.DB.BindNamed(sql, mapArgs)
	if err != nil {
		return nil, errors.Wrap(err, "failed to rebind args")
	}

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
