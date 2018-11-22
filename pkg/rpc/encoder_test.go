package rpc_test

import (
	"context"
	"errors"
	"os"
	"testing"

	kitlog "github.com/go-kit/kit/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	encoder "github.com/thingful/twirp-encoder-go"

	"github.com/DECODEproject/iotencoder/pkg/mocks"
	"github.com/DECODEproject/iotencoder/pkg/postgres"
	"github.com/DECODEproject/iotencoder/pkg/rpc"
	"github.com/DECODEproject/iotencoder/pkg/system"
)

type EncoderTestSuite struct {
	suite.Suite

	db *postgres.DB
}

func (e *EncoderTestSuite) SetupTest() {
	logger := kitlog.NewNopLogger()
	connStr := os.Getenv("IOTENCODER_DATABASE_URL")

	db, err := postgres.Open(connStr)
	if err != nil {
		e.T().Fatalf("Failed to open new connection for migrations: %v", err)
	}

	err = postgres.MigrateDownAll(db.DB, logger)
	if err != nil {
		e.T().Fatalf("Failed to migrate down: %v", err)
	}

	err = postgres.MigrateUp(db.DB, logger)
	if err != nil {
		e.T().Fatalf("Failed to migrate up: %v", err)
	}

	err = db.Close()
	if err != nil {
		e.T().Fatalf("Failed to close db: %v", err)
	}

	e.db = postgres.NewDB(
		&postgres.Config{
			ConnStr:            connStr,
			EncryptionPassword: "password",
			HashidSalt:         "salt",
			HashidMinLength:    8,
		},
		logger,
	)

	e.db.Start()
}

func (e *EncoderTestSuite) TearDownTest() {
	e.db.Stop()
}

func (e *EncoderTestSuite) TestStreamLifecycle() {
	logger := kitlog.NewNopLogger()
	mqttClient := mocks.NewMQTTClient(nil)
	processor := mocks.NewProcessor()

	enc := rpc.NewEncoder(&rpc.Config{
		DB:         e.db,
		MQTTClient: mqttClient,
		Processor:  processor,
		Verbose:    false,
		BrokerAddr: "tcp://mqtt.local:1883",
	}, logger)

	assert.Len(e.T(), mqttClient.Subscriptions, 0)

	err := enc.(system.Startable).Start()
	assert.Nil(e.T(), err)
	defer enc.(system.Stoppable).Stop()

	resp, err := enc.CreateStream(context.Background(), &encoder.CreateStreamRequest{
		DeviceToken:        "abc123",
		RecipientPublicKey: "pub_key",
		PolicyId:           "policy-id",
		Location: &encoder.CreateStreamRequest_Location{
			Longitude: -0.024,
			Latitude:  54.24,
		},
		Exposure: encoder.CreateStreamRequest_INDOOR,
	})
	assert.Nil(e.T(), err)

	assert.Len(e.T(), mqttClient.Subscriptions, 1)
	assert.Len(e.T(), mqttClient.Subscriptions["tcp://mqtt.local:1883"], 1)
	assert.NotEqual(e.T(), "", resp.StreamUid)

	device, err := e.db.GetDevice("abc123")
	assert.Nil(e.T(), err)
	assert.Equal(e.T(), "tcp://mqtt.local:1883", device.Broker)
	assert.Len(e.T(), device.Streams, 1)

	_, err = enc.DeleteStream(context.Background(), &encoder.DeleteStreamRequest{
		StreamUid: resp.StreamUid,
		Token:     resp.Token,
	})
	assert.Nil(e.T(), err)

	device, err = e.db.GetDevice("abc123")
	assert.NotNil(e.T(), err)
}

func (e *EncoderTestSuite) TestSubscriptionsCreatedOnStart() {
	logger := kitlog.NewNopLogger()
	mqttClient := mocks.NewMQTTClient(nil)
	processor := mocks.NewProcessor()

	// insert two streams with devices
	_, err := e.db.CreateStream(&postgres.Stream{
		PublicKey: "abc123",
		PolicyID:  "policy-id",
		Device: &postgres.Device{
			Broker:      "tcp://broker1:1883",
			DeviceToken: "foo",
			Longitude:   23,
			Latitude:    23.2,
			Exposure:    "indoor",
		},
	})
	assert.Nil(e.T(), err)

	_, err = e.db.CreateStream(&postgres.Stream{
		PublicKey: "abc123",
		PolicyID:  "policy-id-2",
		Device: &postgres.Device{
			Broker:      "tcp://broker1:1883",
			DeviceToken: "bar",
			Longitude:   23,
			Latitude:    23.2,
			Exposure:    "indoor",
		},
	})
	assert.Nil(e.T(), err)

	enc := rpc.NewEncoder(&rpc.Config{
		DB:         e.db,
		MQTTClient: mqttClient,
		Processor:  processor,
		Verbose:    true,
		BrokerAddr: "tcp://broker1:1883",
	}, logger)

	enc.(system.Startable).Start()

	assert.Len(e.T(), mqttClient.Subscriptions["tcp://broker1:1883"], 2)

	enc.(system.Stoppable).Stop()
}

func (e *EncoderTestSuite) TestCreateStreamInvalid() {
	logger := kitlog.NewNopLogger()
	mqttClient := mocks.NewMQTTClient(nil)
	processor := mocks.NewProcessor()

	enc := rpc.NewEncoder(&rpc.Config{
		DB:         e.db,
		MQTTClient: mqttClient,
		Processor:  processor,
		Verbose:    true,
		BrokerAddr: "tcp://mqtt",
	}, logger)

	enc.(system.Startable).Start()
	defer enc.(system.Stoppable).Stop()

	testcases := []struct {
		label       string
		request     *encoder.CreateStreamRequest
		expectedErr string
	}{
		{
			label: "missing device token",
			request: &encoder.CreateStreamRequest{
				RecipientPublicKey: "pubkey",
				Location: &encoder.CreateStreamRequest_Location{
					Longitude: 32,
					Latitude:  23,
				},
				Exposure: encoder.CreateStreamRequest_INDOOR,
			},
			expectedErr: "twirp error invalid_argument: device_token is required",
		},
		{
			label: "missing policy id",
			request: &encoder.CreateStreamRequest{
				DeviceToken:        "foo",
				RecipientPublicKey: "pubkey",
				Location: &encoder.CreateStreamRequest_Location{
					Longitude: 32,
					Latitude:  23,
				},
				Exposure: encoder.CreateStreamRequest_INDOOR,
			},
			expectedErr: "twirp error invalid_argument: policy_id is required",
		},
		{
			label: "missing public key",
			request: &encoder.CreateStreamRequest{
				DeviceToken: "foo",
				PolicyId:    "policy-id",
				Location: &encoder.CreateStreamRequest_Location{
					Longitude: 32,
					Latitude:  23,
				},
				Exposure: encoder.CreateStreamRequest_INDOOR,
			},
			expectedErr: "twirp error invalid_argument: recipient_public_key is required",
		},
		{
			label: "missing location",
			request: &encoder.CreateStreamRequest{
				DeviceToken:        "foo",
				PolicyId:           "policy-id",
				RecipientPublicKey: "pubkey",
				Exposure:           encoder.CreateStreamRequest_INDOOR,
			},
			expectedErr: "twirp error invalid_argument: location is required",
		},
		{
			label: "missing longitude",
			request: &encoder.CreateStreamRequest{
				DeviceToken:        "foo",
				PolicyId:           "policy-id",
				RecipientPublicKey: "pubkey",
				Location: &encoder.CreateStreamRequest_Location{
					Latitude: 23,
				},
				Exposure: encoder.CreateStreamRequest_INDOOR,
			},
			expectedErr: "twirp error invalid_argument: longitude is required",
		},
		{
			label: "missing latitude",
			request: &encoder.CreateStreamRequest{
				DeviceToken:        "foo",
				PolicyId:           "policy-id",
				RecipientPublicKey: "pubkey",
				Location: &encoder.CreateStreamRequest_Location{
					Longitude: 45,
				},
				Exposure: encoder.CreateStreamRequest_INDOOR,
			},
			expectedErr: "twirp error invalid_argument: latitude is required",
		},
	}

	for _, tc := range testcases {
		e.T().Run(tc.label, func(t *testing.T) {
			_, err := enc.CreateStream(context.Background(), tc.request)
			assert.NotNil(t, err)
			assert.Equal(t, tc.expectedErr, err.Error())
		})
	}
}

func (e *EncoderTestSuite) TestDeleteStreamInvalid() {
	logger := kitlog.NewNopLogger()
	mqttClient := mocks.NewMQTTClient(nil)
	processor := mocks.NewProcessor()

	enc := rpc.NewEncoder(&rpc.Config{
		DB:         e.db,
		MQTTClient: mqttClient,
		Processor:  processor,
		Verbose:    true,
		BrokerAddr: "tcp://mqtt:1883",
	}, logger)

	enc.(system.Startable).Start()
	defer enc.(system.Stoppable).Stop()

	testcases := []struct {
		label       string
		request     *encoder.DeleteStreamRequest
		expectedErr string
	}{
		{
			label:       "missing stream_uid",
			request:     &encoder.DeleteStreamRequest{Token: "foobar"},
			expectedErr: "twirp error invalid_argument: stream_uid is required",
		},
		{
			label:       "missing token",
			request:     &encoder.DeleteStreamRequest{StreamUid: "foobar"},
			expectedErr: "twirp error invalid_argument: token is required",
		},
		{
			label:       "missing stream",
			request:     &encoder.DeleteStreamRequest{StreamUid: "Gzmdv8vp", Token: "barfoo"},
			expectedErr: "twirp error internal: failed to delete stream: sql: no rows in result set",
		},
	}

	for _, tc := range testcases {
		e.T().Run(tc.label, func(t *testing.T) {
			_, err := enc.DeleteStream(context.Background(), tc.request)
			assert.NotNil(t, err)
			assert.Equal(t, tc.expectedErr, err.Error())
		})
	}
}

func (e *EncoderTestSuite) TestSubscribeErrorContinues() {
	logger := kitlog.NewNopLogger()
	mqttClient := mocks.NewMQTTClient(errors.New("failed"))
	processor := mocks.NewProcessor()

	_, err := e.db.CreateStream(&postgres.Stream{
		PublicKey: "abc123",
		PolicyID:  "policy-id",
		Device: &postgres.Device{
			Broker:      "tcp://broker:1883",
			DeviceToken: "foo",
			Longitude:   23,
			Latitude:    45,
			Exposure:    "indoor",
		},
	})
	assert.Nil(e.T(), err)

	enc := rpc.NewEncoder(&rpc.Config{
		DB:         e.db,
		MQTTClient: mqttClient,
		Processor:  processor,
		Verbose:    true,
		BrokerAddr: "tcp://broker:1883",
	}, logger)

	err = enc.(system.Startable).Start()
	assert.Nil(e.T(), err)
}

func TestRunEncoderTestSuite(t *testing.T) {
	suite.Run(t, new(EncoderTestSuite))
}
