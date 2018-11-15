package postgres

import (
	kitlog "github.com/go-kit/kit/log"
	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"
	"github.com/speps/go-hashids"

	// blank import for db driver
	_ "github.com/lib/pq"
)

const (
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
	PolicyID  string `db:"policy_id"`
	PublicKey string `db:"public_key"`

	StreamID string
	Token    string

	Device *Device
}

// DB is our interface to Postgres. Exposes methods for inserting a new Device
// (and associated Stream), listing all Devices, getting an individual Device,
// and deleting a Stream
type DB interface {
	// CreateStream takes as input a pointer to an instantiated Stream instance
	// which must have a an associated Device. We upsert the Device, then attempt
	// to insert the Stream. Returns a Stream instance with the a string id for the inserted Stream or an
	// error if any failure happens.
	CreateStream(stream *Stream) (*Stream, error)

	// DeleteStream takes as input a string representing the id of a previously
	// stored stream, and attempts to delete the associated stream from the
	// underlying database. If this stream is the last stream associated with a
	// device then the device record is also deleted. We return a Device instance
	// as we need to pass back out the device_token so that we can also
	// unsubscribe from the MQTT topic.
	DeleteStream(stream *Stream) (*Device, error)

	// GetDevices returns a slice containing all Devices currently stored in the
	// system. We know we have a maximum limit here of around 25 devices, so we
	// don't need to worry about this being an excessively large dataset. Note we
	// do not load all Streams here, we just return the device.
	GetDevices() ([]*Device, error)

	// GetDevice returns a single Device with all associated Streams on being given
	// the devices unique token. This is used on boot to create MQTT subscriptions
	// for all Streams.
	GetDevice(deviceToken string) (*Device, error)

	// MigrateUp is a helper method that attempts to run all up migrations against
	// the underlying Postgres DB or returns an error.
	MigrateUp() error
}

// Open is a helper function that takes as input a connection string for a DB,
// and returns either a sqlx.DB instance or an error. This function is separated
// out to help with CLI tasks for managing migrations.
func Open(connStr string) (*sqlx.DB, error) {
	return sqlx.Open("postgres", connStr)
}

// db is our type that wraps an sqlx.DB instance and provides an API for the
// data access functions we require.
type db struct {
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
func NewDB(config *Config, logger kitlog.Logger) DB {
	logger = kitlog.With(logger, "module", "postgres")

	logger.Log("msg", "creating DB instance", "hashidlength", config.HashidMinLength)

	hd := hashids.NewData()
	hd.Salt = config.HashidSalt
	hd.MinLength = config.HashidMinLength

	return &db{
		connStr:            config.ConnStr,
		encryptionPassword: []byte(config.EncryptionPassword),
		hashidData:         hd,
		logger:             logger,
	}
}

// Start creates our DB connection pool running returning an error if any
// failure occurs.
func (d *db) Start() error {
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
func (d *db) Stop() error {
	d.logger.Log("msg", "stopping postgres")

	return d.DB.Close()
}

// CreateStream attempts to insert records into the database for the given
// Stream object. Returns a string containing the ID of the created stream if
// successful or an error if any data constraint is violated, or any other error
// occurs.
func (d *db) CreateStream(stream *Stream) (_ *Stream, err error) {
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
	(device_id, policy_id, public_key, token)
	VALUES (:device_id, :policy_id, :public_key, pgp_sym_encrypt(:token, :encryption_password))
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
func (d *db) DeleteStream(stream *Stream) (_ *Device, err error) {
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
func (d *db) GetDevices() ([]*Device, error) {
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
func (d *db) GetDevice(deviceToken string) (_ *Device, err error) {
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
	sql = `SELECT public_key FROM streams WHERE device_id = :device_id`

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

	err = tx.Map(sql, []interface{}{}, mapper)
	if err != nil {
		return nil, errors.Wrap(err, "failed to execute row mapper")
	}

	device.Streams = streams

	return &device, nil
}

// MigrateUp is a convenience function to run all up migrations in the context
// of an instantiated DB instance.
func (d *db) MigrateUp() error {
	return MigrateUp(d.DB.DB, d.logger)
}
