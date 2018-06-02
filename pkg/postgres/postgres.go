package postgres

import (
	kitlog "github.com/go-kit/kit/log"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"github.com/pkg/errors"
	"github.com/speps/go-hashids"
)

// Device is a type used when reading data back from the DB.
type Device struct {
	ID          int     `db:"id"`
	Broker      string  `db:"broker"`
	Topic       string  `db:"topic"`
	PrivateKey  string  `db:"private_key"`
	UserUID     string  `db:"user_uid"`
	Longitude   float64 `db:"longitude"`
	Latitude    float64 `db:"latitude"`
	Disposition string  `db:"disposition"`

	Streams []*Stream
}

// Stream is a type used when reading data back from the DB, and when creating a
// stream. It contains a public key field used when reading data, and for
// creating a new stream has an associated Device instance.
type Stream struct {
	PublicKey string `db:"public_key"`

	Device *Device
}

// DB is our interface to Postgres. Exposes methods for inserting a new Device
// (and associated Stream), listing all Devices, getting an individual Device,
// and deleting a Stream
type DB interface {
	// CreateStream takes as input a pointer to an instantiated Stream instance
	// which must have a an associated Device. We upsert the Device, then attempt
	// to insert the Stream. Returns a string id for the inserted Stream or an
	// error if any failure happens.
	CreateStream(stream *Stream) (string, error)

	// DeleteStream takes as input a string representing the id of a previously
	// stored stream, and attempts to delete the associated stream from the
	// underlying database. If this stream is the last stream associated with a
	// device then the device record is also deleted. We return a Device instance
	// as we need to pass back out the broker and topic so that we can also
	// unsubscribe from the MQTT topic.
	DeleteStream(id string) (*Device, error)

	// GetDevices returns a slice containing all Devices currently stored in the
	// system. We know we have a maximum limit here of around 25 devices, so we
	// don't need to worry about this being an excessively large dataset. Note we
	// do not load all Streams here, we just return the device.
	GetDevices() ([]*Device, error)

	// GetDevice returns a single Device with all associated Streams on being given
	// the devices unique token. This is used on boot to create MQTT subscriptions
	// for all Streams.
	GetDevice(token string) (*Device, error)

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
func (d *db) CreateStream(stream *Stream) (_ string, err error) {
	// device upsert sql - note we encrypt the private key using postgres native
	// symmetric encryption scheme
	sql := `INSERT INTO devices
		(broker, topic, private_key, user_uid, longitude, latitude, disposition)
	VALUES (:broker, :topic, pgp_sym_encrypt(:private_key, :encryption_password),
		  :user_uid, :longitude, :latitude, :disposition)
	ON CONFLICT (topic) DO UPDATE
	SET broker = EXCLUDED.broker,
			longitude = EXCLUDED.longitude,
			latitude = EXCLUDED.latitude,
			disposition = EXCLUDED.disposition
	RETURNING id`

	mapArgs := map[string]interface{}{
		"broker":              stream.Device.Broker,
		"topic":               stream.Device.Topic,
		"private_key":         stream.Device.PrivateKey,
		"user_uid":            stream.Device.UserUID,
		"longitude":           stream.Device.Longitude,
		"latitude":            stream.Device.Latitude,
		"disposition":         stream.Device.Disposition,
		"encryption_password": d.encryptionPassword,
	}

	tx, err := BeginTX(d.DB)
	if err != nil {
		return "", errors.Wrap(err, "failed to start transaction when inserting device")
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
		return "", errors.Wrap(err, "failed to upsert device")
	}

	// streams insert sql - again worth noting here we encrypt the public key using
	// postgres native pgcrypto symmetric encryption, but also store a hash of the
	// public key allowing us to enforce the uniqueness index on the table without
	// having the unencrypted key written to the disk.
	sql = `INSERT INTO streams
		(device_id, public_key, public_key_hash)
	VALUES (
		:device_id,
		pgp_sym_encrypt(:public_key, :encryption_password),
		digest(:public_key, 'sha256')
	)
	RETURNING id`

	mapArgs = map[string]interface{}{
		"device_id":           deviceID,
		"public_key":          stream.PublicKey,
		"encryption_password": d.encryptionPassword,
	}

	var streamID int

	// again use a Get to get back the stream's id
	err = tx.Get(&streamID, sql, mapArgs)
	if err != nil {
		return "", errors.Wrap(err, "failed to insert stream")
	}

	id, err := d.hashid.Encode([]int{streamID})
	if err != nil {
		return "", errors.Wrap(err, "failed to hash new stream ID")
	}

	return id, err
}

// DeleteStream deletes a stream identified by the given id string. If this
// stream is the last one associated with a device, then the device record is
// also deleted. We return a Device object purely so we can pass back out the
// broker and topic allowing us to unsubscribe.V
func (d *db) DeleteStream(id string) (_ *Device, err error) {
	sql := `DELETE FROM streams WHERE id = :id
		RETURNING device_id`

	streamIDs, err := d.hashid.DecodeWithError(id)
	if err != nil {
		return nil, errors.Wrap(err, "failed to decode hashed id")
	}

	mapArgs := map[string]interface{}{
		"id": streamIDs[0],
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
	sql = `SELECT COUNT(*) FROM streams
		WHERE device_id = :device_id`

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
		sql = `DELETE FROM devices WHERE id = :id
			RETURNING broker, topic`

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
// 25. Note we do not load all streams for these devices.
func (d *db) GetDevices() ([]*Device, error) {
	sql := `SELECT id, broker, topic, pgp_sym_decrypt(private_key, :encryption_password) AS private_key
		FROM devices`

	mapArgs := map[string]interface{}{
		"encryption_password": d.encryptionPassword,
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

	err = tx.Map(sql, mapArgs, mapper)
	if err != nil {
		return nil, errors.Wrap(err, "failed to select device rows from database")
	}

	return devices, nil
}

// GetDevice returns a single device identified by topic, including all streams
// for that device. This is used to set up subscriptions for existing records on
// application start.
func (d *db) GetDevice(topic string) (_ *Device, err error) {
	sql := `SELECT id, broker, topic, pgp_sym_decrypt(private_key, :encryption_password) AS private_key,
	  user_uid, longitude, latitude, disposition
		FROM devices
		WHERE topic = :topic`

	mapArgs := map[string]interface{}{
		"encryption_password": d.encryptionPassword,
		"topic":               topic,
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
	sql = `SELECT pgp_sym_decrypt(public_key, :encryption_password) AS public_key
		FROM streams
		WHERE device_id = :device_id`

	mapArgs = map[string]interface{}{
		"encryption_password": d.encryptionPassword,
		"device_id":           device.ID,
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
func (d *db) MigrateUp() error {
	return MigrateUp(d.DB.DB, d.logger)
}
