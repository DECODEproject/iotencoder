package postgres

import (
	"database/sql/driver"
	"encoding/json"

	kitlog "github.com/go-kit/kit/log"
	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"
	"github.com/speps/go-hashids"

	// blank import for db driver
	_ "github.com/lib/pq"
)

// Action is a type alias for string - we use for constants
type Action string

const (
	// Share defines an action of sharing a sensor without processing
	Share Action = "SHARE"

	// Bin defines an action of sharing binned values for a sensor
	Bin Action = "BIN"

	// MovingAverage defines an action of sharing a moving average for a sensor
	MovingAverage Action = "MOVING_AVG"

	// TokenLength is a constant which controls the length in bytes of the security
	// tokens we generate for streams.
	TokenLength = 24
)

// Device is a type used when reading data back from the DB. A single Device may
// feed data to multiple streams, hence the separation here with the associated
// Stream type.
type Device struct {
	ID          int     `db:"id"`
	Broker      string  `db:"broker"`
	DeviceToken string  `db:"device_token"`
	Longitude   float64 `db:"longitude"`
	Latitude    float64 `db:"latitude"`
	Exposure    string  `db:"exposure"`

	Streams []*Stream
}

// Stream is a type used when reading data back from the DB, and when creating a
// stream. It contains a public key field used when reading data, and for
// creating a new stream has an associated Device instance.
type Stream struct {
	PolicyID   string     `db:"policy_id"`
	PublicKey  string     `db:"public_key"`
	Operations Operations `db:"operations"`

	StreamID string
	Token    string

	Device *Device
}

// Operation is a type used to capture the data around the operations to be
// applied to a Stream.
type Operation struct {
	SensorID uint32    `json:"sensorId"`
	Action   Action    `json:"action"`
	Bins     []float64 `json:"bins"`
	Interval uint32    `json:"interval"`
}

// Operations is a type alias for a slice of Operation instance. We add as a
// separate type as we implement sql.Valuer and sql.Scanner interfaces to read
// and write back from the DB.
type Operations []*Operation

// Value is our implementation of the sql.Valuer interface which converts the
// instance into a value that can be written to the database.
func (o Operations) Value() (driver.Value, error) {
	return json.Marshal(o)
}

// Scan is our implementation of the sql.Scanner interface which takes the value
// read from the database, and converts it back into an instance of the type.
func (o *Operations) Scan(src interface{}) error {
	if o == nil {
		return nil
	}

	source, ok := src.([]byte)
	if !ok {
		return errors.New("Value read from database cannot be typecast to a byte slice")
	}

	err := json.Unmarshal(source, &o)
	if err != nil {
		return errors.Wrap(err, "failed to unmarshal bytes into Operations")
	}

	return nil
}

// Open is a helper function that takes as input a connection string for a DB,
// and returns either a sqlx.DB instance or an error. This function is separated
// out to help with CLI tasks for managing migrations.
func Open(connStr string) (*sqlx.DB, error) {
	return sqlx.Open("postgres", connStr)
}

// DB is our type that wraps an sqlx.DB instance and provides an API for the
// data access functions we require.
type DB struct {
	connStr            string
	encryptionPassword []byte
	hashidData         *hashids.HashIDData
	DB                 *sqlx.DB
	hashid             *hashids.HashID
	logger             kitlog.Logger
}

// Config is used to carry package local configuration for Postgres DB module.
type Config struct {
	ConnStr            string
	EncryptionPassword string
	HashidSalt         string
	HashidMinLength    int
}

// NewDB creates a new DB instance with the given connection string. We also
// pass in a logger.
func NewDB(config *Config, logger kitlog.Logger) *DB {
	logger = kitlog.With(logger, "module", "postgres")

	hd := hashids.NewData()
	hd.Salt = config.HashidSalt
	hd.MinLength = config.HashidMinLength

	return &DB{
		connStr:            config.ConnStr,
		encryptionPassword: []byte(config.EncryptionPassword),
		hashidData:         hd,
		logger:             logger,
	}
}

// Start creates our DB connection pool running returning an error if any
// failure occurs.
func (d *DB) Start() error {
	d.logger.Log("msg", "starting postgres")

	db, err := Open(d.connStr)
	if err != nil {
		return errors.Wrap(err, "opening db connection failed")
	}

	h, err := hashids.NewWithData(d.hashidData)
	if err != nil {
		return errors.Wrap(err, "creating hashid generator failed")
	}

	d.DB = db
	d.hashid = h

	return nil
}

// Stop closes the DB connection pool.
func (d *DB) Stop() error {
	d.logger.Log("msg", "stopping postgres client")

	return d.DB.Close()
}

// CreateStream attempts to insert records into the database for the given
// Stream object. Returns a string containing the ID of the created stream if
// successful or an error if any data constraint is violated, or any other error
// occurs.
func (d *DB) CreateStream(stream *Stream) (_ *Stream, err error) {
	sql := `INSERT INTO devices
		(broker, device_token, longitude, latitude, exposure)
	VALUES (:broker, :device_token, :longitude, :latitude, :exposure)
	ON CONFLICT (device_token) DO UPDATE
	SET broker = EXCLUDED.broker,
			longitude = EXCLUDED.longitude,
			latitude = EXCLUDED.latitude,
			exposure = EXCLUDED.exposure
	RETURNING id`

	mapArgs := map[string]interface{}{
		"broker":       stream.Device.Broker,
		"device_token": stream.Device.DeviceToken,
		"longitude":    stream.Device.Longitude,
		"latitude":     stream.Device.Latitude,
		"exposure":     stream.Device.Exposure,
	}

	tx, err := BeginTX(d.DB)
	if err != nil {
		return nil, errors.Wrap(err, "failed to start transaction when inserting device")
	}

	defer func() {
		if cerr := tx.CommitOrRollback(); err == nil && cerr != nil {
			err = cerr
		}
	}()

	var deviceID int

	// we use a Get for the upsert so we get back the device id
	err = tx.Get(&deviceID, sql, mapArgs)
	if err != nil {
		return nil, errors.Wrap(err, "failed to upsert device")
	}

	// streams insert sql
	sql = `INSERT INTO streams
	(device_id, policy_id, public_key, token, operations)
	VALUES (:device_id, :policy_id, :public_key, pgp_sym_encrypt(:token, :encryption_password), :operations)
	RETURNING id`

	token, err := GenerateToken(TokenLength)
	if err != nil {
		return nil, errors.Wrap(err, "failed to generate random token")
	}

	mapArgs = map[string]interface{}{
		"device_id":           deviceID,
		"policy_id":           stream.PolicyID,
		"public_key":          stream.PublicKey,
		"token":               token,
		"encryption_password": d.encryptionPassword,
		"operations":          stream.Operations,
	}

	var streamID int

	// again use a Get to get back the stream's id
	err = tx.Get(&streamID, sql, mapArgs)
	if err != nil {
		return nil, errors.Wrap(err, "failed to insert stream")
	}

	id, err := d.hashid.Encode([]int{streamID})
	if err != nil {
		return nil, errors.Wrap(err, "failed to hash new stream ID")
	}

	stream.StreamID = id
	stream.Token = token

	return stream, err
}

// DeleteStream deletes a stream identified by the given id string. If this
// stream is the last one associated with a device, then the device record is
// also deleted. We return a Device object purely so we can pass back out the
// broker and token allowing us to unsubscribe.
func (d *DB) DeleteStream(stream *Stream) (_ *Device, err error) {
	sql := `DELETE FROM streams
	WHERE id = :id
	AND pgp_sym_decrypt(token, :encryption_password) = :token
	RETURNING device_id`

	streamIDs, err := d.hashid.DecodeWithError(stream.StreamID)
	if err != nil {
		return nil, errors.Wrap(err, "failed to decode hashed id")
	}

	mapArgs := map[string]interface{}{
		"id":                  streamIDs[0],
		"encryption_password": d.encryptionPassword,
		"token":               stream.Token,
	}

	tx, err := BeginTX(d.DB)
	if err != nil {
		return nil, errors.Wrap(err, "failed to start transaction when deleting stream")
	}

	defer func() {
		if cerr := tx.CommitOrRollback(); err == nil && cerr != nil {
			err = cerr
		}
	}()

	var deviceID int

	// again use a Get to run the delete so we get back the device's id
	err = tx.Get(&deviceID, sql, mapArgs)
	if err != nil {
		return nil, errors.Wrap(err, "failed to delete stream")
	}

	// now we count streams for that device id, and if no more we should also
	// delete the device and unsubscribe from its topic
	sql = `SELECT COUNT(*) FROM streams WHERE device_id = :device_id`

	mapArgs = map[string]interface{}{
		"device_id": deviceID,
	}

	var streamCount int

	// again use a Get to get the count
	err = tx.Get(&streamCount, sql, mapArgs)
	if err != nil {
		return nil, errors.Wrap(err, "failed to count streams")
	}

	if streamCount == 0 {
		// delete the device too
		sql = `DELETE FROM devices WHERE id = :id RETURNING broker, device_token`

		mapArgs = map[string]interface{}{
			"id": deviceID,
		}

		var device Device

		err = tx.Get(&device, sql, mapArgs)
		if err != nil {
			return nil, errors.Wrap(err, "failed to delete device")
		}

		return &device, nil
	}

	return nil, nil
}

// GetDevices returns a slice of pointers to Device instances. We don't worry
// about pagination here as we have a maximum number of devices of approximately
// 25 to 50. Note we do not load all streams for these devices.
func (d *DB) GetDevices() ([]*Device, error) {
	sql := `SELECT id, broker, device_token FROM devices`

	tx, err := BeginTX(d.DB)
	if err != nil {
		return nil, errors.Wrap(err, "failed to begin transaction")
	}

	defer func() {
		if cerr := tx.CommitOrRollback(); err == nil && cerr != nil {
			err = cerr
		}
	}()

	devices := []*Device{}

	mapper := func(rows *sqlx.Rows) error {
		for rows.Next() {
			var d Device

			err = rows.StructScan(&d)
			if err != nil {
				return errors.Wrap(err, "failed to scan row into Device struct")
			}

			devices = append(devices, &d)
		}

		return nil
	}

	err = tx.Map(sql, []interface{}{}, mapper)
	if err != nil {
		return nil, errors.Wrap(err, "failed to select device rows from database")
	}

	return devices, nil
}

// GetDevice returns a single device identified by device_token, including all streams
// for that device. This is used to set up subscriptions for existing records on
// application start.
func (d *DB) GetDevice(deviceToken string) (_ *Device, err error) {
	sql := `SELECT id, broker, device_token, longitude, latitude, exposure
		FROM devices
		WHERE device_token = :device_token`

	mapArgs := map[string]interface{}{
		"device_token": deviceToken,
	}

	tx, err := BeginTX(d.DB)
	if err != nil {
		return nil, errors.Wrap(err, "failed to begin transaction")
	}

	defer func() {
		if cerr := tx.CommitOrRollback(); err == nil && cerr != nil {
			err = cerr
		}
	}()

	var device Device
	err = tx.Get(&device, sql, mapArgs)
	if err != nil {
		return nil, errors.Wrap(err, "failed to load device")
	}

	// now load streams
	sql = `SELECT policy_id, public_key, operations FROM streams WHERE device_id = :device_id`

	mapArgs = map[string]interface{}{
		"device_id": device.ID,
	}

	streams := []*Stream{}

	mapper := func(rows *sqlx.Rows) error {

		for rows.Next() {
			var s Stream

			err = rows.StructScan(&s)
			if err != nil {
				return errors.Wrap(err, "failed to scan stream row into struct")
			}

			streams = append(streams, &s)
		}

		return nil
	}

	err = tx.Map(sql, mapArgs, mapper)
	if err != nil {
		return nil, errors.Wrap(err, "failed to execute row mapper")
	}

	device.Streams = streams

	return &device, nil
}

// MigrateUp is a convenience function to run all up migrations in the context
// of an instantiated DB instance.
func (d *DB) MigrateUp() error {
	return MigrateUp(d.DB.DB, d.logger)
}

// Ping attempts to verify the database connection is still alive by executing a
// simple select query on the database server. We don't use the built in
// DB.Ping() function here as this may not go to the database if there existing
// connections in the pool.
func (d *DB) Ping() error {
	_, err := d.DB.Exec("SELECT 1")
	if err != nil {
		return err
	}
	return nil
}
