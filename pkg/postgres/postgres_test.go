package postgres_test

import (
	"context"
	"os"
	"testing"

	kitlog "github.com/go-kit/kit/log"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"golang.org/x/crypto/acme/autocert"

	"github.com/DECODEproject/iotencoder/pkg/postgres"
)

type PostgresSuite struct {
	suite.Suite
	db *postgres.DB
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
		},
		logger,
	)

	s.db.Start()
}

func (s *PostgresSuite) TearDownTest() {
	s.db.Stop()
}

func (s *PostgresSuite) TestRoundTrip() {
	stream1, err := s.db.CreateStream(&postgres.Stream{
		CommunityID: "policy-id",
		PublicKey:   "public",
		Device: &postgres.Device{
			DeviceToken: "123",
			Longitude:   45.2,
			Latitude:    23.2,
			Exposure:    "indoor",
		},
	})

	assert.Nil(s.T(), err)
	assert.NotEqual(s.T(), "", stream1.StreamID)
	assert.NotEqual(s.T(), "", stream1.Token)

	stream2, err := s.db.CreateStream(&postgres.Stream{
		CommunityID: "policy-id",
		PublicKey:   "public",
		Device: &postgres.Device{
			DeviceToken: "124",
			Longitude:   45.2,
			Latitude:    23.2,
			Exposure:    "indoor",
		},
	})

	assert.Nil(s.T(), err)
	assert.NotEqual(s.T(), "", stream2.StreamID)
	assert.NotEqual(s.T(), "", stream2.Token)

	devices, err := s.db.GetDevices()
	assert.Nil(s.T(), err)
	assert.Len(s.T(), devices, 2)

	assert.Equal(s.T(), "123", devices[0].DeviceToken)

	assert.Equal(s.T(), "124", devices[1].DeviceToken)

	device, err := s.db.GetDevice("123")
	assert.Nil(s.T(), err)
	assert.NotNil(s.T(), device)

	assert.Equal(s.T(), "123", device.DeviceToken)
	assert.Equal(s.T(), 45.2, device.Longitude)
	assert.Equal(s.T(), 23.2, device.Latitude)
	assert.Equal(s.T(), "indoor", device.Exposure)
	assert.Len(s.T(), device.Streams, 1)
	assert.Equal(s.T(), "public", device.Streams[0].PublicKey)
	assert.Equal(s.T(), "policy-id", device.Streams[0].CommunityID)

	device, err = s.db.DeleteStream(stream1)
	assert.Nil(s.T(), err)
	assert.Equal(s.T(), "123", device.DeviceToken)

	devices, err = s.db.GetDevices()
	assert.Nil(s.T(), err)
	assert.Len(s.T(), devices, 1)
}

func (s *PostgresSuite) TestRoundTripWithOperations() {
	stream, err := s.db.CreateStream(&postgres.Stream{
		CommunityID: "policy-id",
		PublicKey:   "public",
		Operations: []*postgres.Operation{
			&postgres.Operation{
				SensorID: 12,
				Action:   "SHARE",
			},
		},
		Device: &postgres.Device{
			DeviceToken: "123",
			Longitude:   45.2,
			Latitude:    23.2,
			Exposure:    "indoor",
		},
	})

	assert.Nil(s.T(), err)
	assert.NotEqual(s.T(), "", stream.StreamID)
	assert.Len(s.T(), stream.Operations, 1)

	device, err := s.db.GetDevice("123")
	assert.Nil(s.T(), err)
	assert.NotNil(s.T(), device)

	readStream := device.Streams[0]
	assert.Len(s.T(), readStream.Operations, 1)
}

func (s *PostgresSuite) TestInvalidDeleteStream() {
	unknownStreamID := uuid.New().String()

	stream, err := s.db.CreateStream(&postgres.Stream{
		CommunityID: "policy-id",
		PublicKey:   "public",
		Device: &postgres.Device{
			DeviceToken: "123",
			Longitude:   45.2,
			Latitude:    23.2,
			Exposure:    "indoor",
		},
	})
	assert.Nil(s.T(), err)

	testcases := []struct {
		label       string
		stream      *postgres.Stream
		expectedErr string
	}{
		{
			"incorrect stream id",
			&postgres.Stream{StreamID: unknownStreamID, Token: stream.Token},
			"failed to delete stream: sql: no rows in result set",
		},
		{
			"incorrect token",
			&postgres.Stream{StreamID: stream.StreamID, Token: "foobar"},
			"failed to delete stream: sql: no rows in result set",
		},
	}

	for _, tc := range testcases {
		s.T().Run(tc.label, func(t *testing.T) {
			_, err := s.db.DeleteStream(tc.stream)
			assert.NotNil(t, err)
			assert.Equal(t, tc.expectedErr, err.Error())
		})
	}
}

func (s *PostgresSuite) TestDeleteStreamLeavesDeviceIfOtherStreams() {
	stream1, err := s.db.CreateStream(&postgres.Stream{
		PublicKey:   "public1",
		CommunityID: "policy-id1",
		Device: &postgres.Device{
			DeviceToken: "foo",
			Longitude:   45.2,
			Latitude:    23.2,
			Exposure:    "indoor",
		},
	})

	assert.Nil(s.T(), err)
	assert.NotEqual(s.T(), "", stream1.StreamID)

	stream2, err := s.db.CreateStream(&postgres.Stream{
		PublicKey:   "public2",
		CommunityID: "policy-id2",
		Device: &postgres.Device{
			DeviceToken: "foo",
			Longitude:   45.2,
			Latitude:    23.2,
			Exposure:    "indoor",
		},
	})

	assert.Nil(s.T(), err)
	assert.NotEqual(s.T(), "", stream2.StreamID)

	devices, err := s.db.GetDevices()
	assert.Nil(s.T(), err)
	assert.Len(s.T(), devices, 1)

	_, err = s.db.DeleteStream(stream1)
	assert.Nil(s.T(), err)

	devices, err = s.db.GetDevices()
	assert.Nil(s.T(), err)
	assert.Len(s.T(), devices, 1)
}

func (s *PostgresSuite) TestStreamDeviceRecipientUniqueness() {
	_, err := s.db.CreateStream(&postgres.Stream{
		PublicKey:   "public",
		CommunityID: "policy-id",
		Device: &postgres.Device{
			DeviceToken: "123",
			Longitude:   45.2,
			Latitude:    23.2,
			Exposure:    "indoor",
		},
	})

	assert.Nil(s.T(), err)

	_, err = s.db.CreateStream(&postgres.Stream{
		PublicKey:   "public",
		CommunityID: "policy-id",
		Device: &postgres.Device{
			DeviceToken: "123",
			Longitude:   45.2,
			Latitude:    23.2,
			Exposure:    "indoor",
		},
	})

	assert.NotNil(s.T(), err)
}

func (s *PostgresSuite) TestCertificates() {
	ctx := context.Background()

	// nonexistent key should return error
	_, err := s.db.Get(ctx, "baz")
	assert.NotNil(s.T(), err)
	assert.Equal(s.T(), autocert.ErrCacheMiss, err)

	// should be able to write a cert
	err = s.db.Put(ctx, "foo", []byte("bar"))
	assert.Nil(s.T(), err)

	// now should be able to read it
	cert, err := s.db.Get(ctx, "foo")
	assert.Nil(s.T(), err)
	assert.Equal(s.T(), []byte("bar"), cert)

	// should be able to delete it
	err = s.db.Delete(ctx, "foo")
	assert.Nil(s.T(), err)

	// now should not be able to read it
	_, err = s.db.Get(ctx, "foo")
	assert.NotNil(s.T(), err)
	assert.Equal(s.T(), autocert.ErrCacheMiss, err)
}

func TestRunPostgresSuite(t *testing.T) {
	suite.Run(t, new(PostgresSuite))
}
