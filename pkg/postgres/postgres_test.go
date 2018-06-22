package postgres_test

import (
	"os"
	"testing"

	kitlog "github.com/go-kit/kit/log"
	"github.com/guregu/null"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	encoder "github.com/thingful/twirp-encoder-go"

	"github.com/thingful/iotencoder/pkg/postgres"
	"github.com/thingful/iotencoder/pkg/system"
)

type PostgresSuite struct {
	suite.Suite
	db postgres.DB
}

func (s *PostgresSuite) SetupTest() {
	logger := kitlog.NewNopLogger()
	connStr := os.Getenv("IOTENCODER_DATABASE_URL")

	db, err := postgres.Open(connStr)
	if err != nil {
		s.T().Fatalf("Failed to open new connection for migrations: %v", err)
	}

	err = postgres.MigrateDownAll(db.DB, logger)
	if err != nil {
		s.T().Fatalf("Failed to migrate down: %v", err)
	}

	err = postgres.MigrateUp(db.DB, logger)
	if err != nil {
		s.T().Fatalf("Failed to migrate up: %v", err)
	}

	err = db.Close()
	if err != nil {
		s.T().Fatalf("Failed to close db: %v", err)
	}

	s.db = postgres.NewDB(
		&postgres.Config{
			ConnStr:            connStr,
			EncryptionPassword: "password",
			HashidSalt:         "salt",
			HashidMinLength:    8,
		},
		logger,
	)

	s.db.(system.Startable).Start()
}

func (s *PostgresSuite) TearDownTest() {
	s.db.(system.Stoppable).Stop()
}

func (s *PostgresSuite) TestRoundTrip() {
	tx, err := s.db.BeginTX()
	assert.Nil(s.T(), err)

	streamID1, err := s.db.CreateStream(tx, &postgres.Stream{
		PublicKey: "public",
		Device: &postgres.Device{
			Broker:     "tcp://example.com",
			Topic:      "device/123",
			PrivateKey: "private",
			UserUID:    "bob",
			Longitude:  45.2,
			Latitude:   23.2,
			Exposure:   "indoor",
		},
		Entitlements: []postgres.Entitlement{
			{
				SensorID: 29,
				Action:   encoder.CreateStreamRequest_Entitlement_MOVING_AVG.String(),
				Interval: null.IntFrom(900),
			},
			{
				SensorID: 14,
				Action:   encoder.CreateStreamRequest_Entitlement_BIN.String(),
				Bins:     []float64{40, 80},
			},
		},
	})

	assert.Nil(s.T(), err)
	assert.NotEqual(s.T(), "", streamID1)

	streamID2, err := s.db.CreateStream(tx, &postgres.Stream{
		PublicKey: "public",
		Device: &postgres.Device{
			Broker:     "tcp://mqtt.com",
			Topic:      "device/124",
			PrivateKey: "private",
			UserUID:    "bob",
			Longitude:  45.2,
			Latitude:   23.2,
			Exposure:   "indoor",
		},
	})

	assert.Nil(s.T(), err)
	assert.NotEqual(s.T(), "", streamID2)

	devices, err := s.db.GetDevices(tx)
	assert.Nil(s.T(), err)
	assert.Len(s.T(), devices, 2)

	assert.Equal(s.T(), "tcp://example.com", devices[0].Broker)
	assert.Equal(s.T(), "device/123", devices[0].Topic)
	assert.Equal(s.T(), "private", devices[0].PrivateKey)

	assert.Equal(s.T(), "tcp://mqtt.com", devices[1].Broker)
	assert.Equal(s.T(), "device/124", devices[1].Topic)
	assert.Equal(s.T(), "private", devices[1].PrivateKey)

	device, err := s.db.GetDevice(tx, "device/123")
	assert.Nil(s.T(), err)

	assert.Equal(s.T(), "tcp://example.com", device.Broker)
	assert.Equal(s.T(), "device/123", device.Topic)
	assert.Equal(s.T(), "private", device.PrivateKey)
	assert.Equal(s.T(), "bob", device.UserUID)
	assert.Equal(s.T(), 45.2, device.Longitude)
	assert.Equal(s.T(), 23.2, device.Latitude)
	assert.Equal(s.T(), "indoor", device.Exposure)
	assert.Len(s.T(), device.Streams, 1)
	assert.Equal(s.T(), "public", device.Streams[0].PublicKey)
	assert.Len(s.T(), device.Streams[0].Entitlements, 2)

	entitlement1 := device.Streams[0].Entitlements[0]
	assert.Equal(s.T(), 29, entitlement1.SensorID)
	assert.Equal(s.T(), "MOVING_AVG", entitlement1.Action)
	assert.Equal(s.T(), int64(900), entitlement1.Interval.Int64)

	entitlement2 := device.Streams[0].Entitlements[1]
	assert.Equal(s.T(), 14, entitlement2.SensorID)
	assert.Equal(s.T(), "BIN", entitlement2.Action)
	assert.Equal(s.T(), []float64{40, 80}, entitlement2.Bins)
	assert.False(s.T(), entitlement2.Interval.Valid)

	device, err = s.db.DeleteStream(tx, streamID1)
	assert.Nil(s.T(), err)
	assert.Equal(s.T(), "tcp://example.com", device.Broker)
	assert.Equal(s.T(), "device/123", device.Topic)

	devices, err = s.db.GetDevices(tx)
	assert.Nil(s.T(), err)
	assert.Len(s.T(), devices, 1)

	err = tx.Rollback()
	assert.Nil(s.T(), err)
}

func (s *PostgresSuite) TestInvalidDelete() {
	tx, err := s.db.BeginTX()
	assert.Nil(s.T(), err)

	testcases := []struct {
		label       string
		streamID    string
		expectedErr string
	}{
		{
			"missing stream",
			"Gzmdv8vp",
			"failed to delete stream: sql: no rows in result set",
		},
		{
			"invalid id",
			"foo",
			"failed to decode hashed id: mismatch between encode and decode: foo start a63Oaakq re-encoded. result: [900]",
		},
	}

	for _, tc := range testcases {
		s.T().Run(tc.label, func(t *testing.T) {
			_, err := s.db.DeleteStream(tx, tc.streamID)
			assert.NotNil(t, err)
			assert.Equal(t, tc.expectedErr, err.Error())
		})
	}

	err = tx.Rollback()
	assert.Nil(s.T(), err)
}

func (s *PostgresSuite) TestDeleteStreamLeavesDeviceIfOtherStreams() {
	tx, err := s.db.BeginTX()
	assert.Nil(s.T(), err)

	streamID1, err := s.db.CreateStream(tx, &postgres.Stream{
		PublicKey: "public1",
		Device: &postgres.Device{
			Broker:     "tcp://example.com",
			Topic:      "device/foo",
			PrivateKey: "private",
			UserUID:    "bob",
			Longitude:  45.2,
			Latitude:   23.2,
			Exposure:   "indoor",
		},
	})

	assert.Nil(s.T(), err)
	assert.NotEqual(s.T(), "", streamID1)

	streamID2, err := s.db.CreateStream(tx, &postgres.Stream{
		PublicKey: "public2",
		Device: &postgres.Device{
			Broker:     "tcp://mqtt.com",
			Topic:      "device/foo",
			PrivateKey: "private",
			UserUID:    "bob",
			Longitude:  45.2,
			Latitude:   23.2,
			Exposure:   "indoor",
		},
	})

	assert.Nil(s.T(), err)
	assert.NotEqual(s.T(), "", streamID2)

	devices, err := s.db.GetDevices(tx)
	assert.Nil(s.T(), err)
	assert.Len(s.T(), devices, 1)

	_, err = s.db.DeleteStream(tx, streamID1)
	assert.Nil(s.T(), err)

	devices, err = s.db.GetDevices(tx)
	assert.Nil(s.T(), err)
	assert.Len(s.T(), devices, 1)

	err = tx.Rollback()
	assert.Nil(s.T(), err)
}

func (s *PostgresSuite) TestStreamDeviceRecipientUniqueness() {
	tx, err := s.db.BeginTX()
	assert.Nil(s.T(), err)

	_, err = s.db.CreateStream(tx, &postgres.Stream{
		PublicKey: "public",
		Device: &postgres.Device{
			Broker:     "tcp://unique.com",
			Topic:      "device/123",
			PrivateKey: "private",
			UserUID:    "bob",
			Longitude:  45.2,
			Latitude:   23.2,
			Exposure:   "indoor",
		},
	})

	assert.Nil(s.T(), err)

	_, err = s.db.CreateStream(tx, &postgres.Stream{
		PublicKey: "public",
		Device: &postgres.Device{
			Broker:     "tcp://unique.com",
			Topic:      "device/123",
			PrivateKey: "private",
			UserUID:    "bob",
			Longitude:  45.2,
			Latitude:   23.2,
			Exposure:   "indoor",
		},
	})

	assert.NotNil(s.T(), err)

	err = tx.Rollback()
	assert.Nil(s.T(), err)
}

func TestRunPostgresSuite(t *testing.T) {
	suite.Run(t, new(PostgresSuite))
}
