package rpc_test

import (
	"context"
	"os"
	"testing"

	kitlog "github.com/go-kit/kit/log"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"

	"github.com/thingful/iotencoder/pkg/mocks"
	"github.com/thingful/iotencoder/pkg/postgres"
	"github.com/thingful/iotencoder/pkg/rpc"
	"github.com/thingful/iotencoder/pkg/system"
	encoder "github.com/thingful/twirp-encoder-go"
)

type EncoderTestSuite struct {
	suite.Suite

	db postgres.DB
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

	e.db.(system.Startable).Start()
}

func (e *EncoderTestSuite) TearDownTest() {
	e.db.(system.Stoppable).Stop()
}

func (e *EncoderTestSuite) TestStreamLifecycle() {
	logger := kitlog.NewNopLogger()
	mqttClient := mocks.NewMQTTClient(nil)
	processor := mocks.NewProcessor()

	tx, err := e.db.BeginTX()
	assert.Nil(e.T(), err)

	enc := rpc.NewEncoder(&rpc.Config{
		DB:         e.db,
		MQTTClient: mqttClient,
		Processor:  processor,
		Verbose:    false,
	}, logger)

	assert.Len(e.T(), mqttClient.Subscriptions, 0)

	err = enc.(system.Startable).Start()
	assert.Nil(e.T(), err)
	defer enc.(system.Stoppable).Stop()

	resp, err := enc.CreateStream(context.Background(), &encoder.CreateStreamRequest{
		BrokerAddress:      "tcp://mqtt.local:1883",
		DeviceTopic:        "device/sck/abc123/readings",
		DevicePrivateKey:   "priv_key",
		RecipientPublicKey: "pub_key",
		UserUid:            "alice",
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

	device, err := e.db.GetDevice(tx, "device/sck/abc123/readings")
	assert.Nil(e.T(), err)
	assert.Equal(e.T(), "tcp://mqtt.local:1883", device.Broker)
	assert.Len(e.T(), device.Streams, 1)

	_, err = enc.DeleteStream(context.Background(), &encoder.DeleteStreamRequest{
		StreamUid: resp.StreamUid,
	})
	assert.Nil(e.T(), err)

	device, err = e.db.GetDevice(tx, "device/sck/abc123/readings")
	assert.NotNil(e.T(), err)

	err = tx.Rollback()
	assert.Nil(e.T(), err)
}

func (e *EncoderTestSuite) TestSubscriptionsCreatedOnStart() {
	logger := kitlog.NewNopLogger()
	mqttClient := mocks.NewMQTTClient(nil)
	processor := mocks.NewProcessor()

	tx, err := e.db.BeginTX()
	assert.Nil(e.T(), err)

	// insert two streams with devices
	_, err = e.db.CreateStream(tx, &postgres.Stream{
		PublicKey: "abc123",
		Device: &postgres.Device{
			Broker:    "tcp://broker1:1883",
			Topic:     "devices/foo",
			UserUID:   "bob",
			Longitude: 23,
			Latitude:  23.2,
			Exposure:  "indoor",
		},
	})
	assert.Nil(e.T(), err)

	_, err = e.db.CreateStream(tx, &postgres.Stream{
		PublicKey: "abc123",
		Device: &postgres.Device{
			Broker:    "tcp://broker1:1883",
			Topic:     "devices/bar",
			UserUID:   "bob",
			Longitude: 23,
			Latitude:  23.2,
			Exposure:  "indoor",
		},
	})
	assert.Nil(e.T(), err)

	err = tx.Commit()
	assert.Nil(e.T(), err)

	enc := rpc.NewEncoder(&rpc.Config{
		DB:         e.db,
		MQTTClient: mqttClient,
		Processor:  processor,
		Verbose:    true,
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
	}, logger)

	enc.(system.Startable).Start()
	defer enc.(system.Stoppable).Stop()

	testcases := []struct {
		label       string
		request     *encoder.CreateStreamRequest
		expectedErr string
	}{
		{
			label: "missing broker",
			request: &encoder.CreateStreamRequest{
				DeviceTopic:        "devices/foo",
				DevicePrivateKey:   "privkey",
				RecipientPublicKey: "pubkey",
				UserUid:            "bob",
				Location: &encoder.CreateStreamRequest_Location{
					Longitude: 32,
					Latitude:  23,
				},
				Exposure: encoder.CreateStreamRequest_INDOOR,
			},
			expectedErr: "twirp error invalid_argument: broker_address is required",
		},
		{
			label: "missing topic",
			request: &encoder.CreateStreamRequest{
				BrokerAddress:      "tcp://mqtt",
				DevicePrivateKey:   "privkey",
				RecipientPublicKey: "pubkey",
				UserUid:            "bob",
				Location: &encoder.CreateStreamRequest_Location{
					Longitude: 32,
					Latitude:  23,
				},
				Exposure: encoder.CreateStreamRequest_INDOOR,
			},
			expectedErr: "twirp error invalid_argument: device_topic is required",
		},
		{
			label: "missing private key",
			request: &encoder.CreateStreamRequest{
				DeviceTopic:        "devices/foo",
				BrokerAddress:      "tcp://mqtt",
				RecipientPublicKey: "pubkey",
				UserUid:            "bob",
				Location: &encoder.CreateStreamRequest_Location{
					Longitude: 32,
					Latitude:  23,
				},
				Exposure: encoder.CreateStreamRequest_INDOOR,
			},
			expectedErr: "twirp error invalid_argument: device_private_key is required",
		},
		{
			label: "missing public key",
			request: &encoder.CreateStreamRequest{
				DeviceTopic:      "devices/foo",
				BrokerAddress:    "tcp://mqtt",
				DevicePrivateKey: "privkey",
				UserUid:          "bob",
				Location: &encoder.CreateStreamRequest_Location{
					Longitude: 32,
					Latitude:  23,
				},
				Exposure: encoder.CreateStreamRequest_INDOOR,
			},
			expectedErr: "twirp error invalid_argument: recipient_public_key is required",
		},
		{
			label: "missing user_uid",
			request: &encoder.CreateStreamRequest{
				DeviceTopic:        "devices/foo",
				BrokerAddress:      "tcp://mqtt",
				DevicePrivateKey:   "privkey",
				RecipientPublicKey: "pubkey",
				Location: &encoder.CreateStreamRequest_Location{
					Longitude: 32,
					Latitude:  23,
				},
				Exposure: encoder.CreateStreamRequest_INDOOR,
			},
			expectedErr: "twirp error invalid_argument: user_uid is required",
		},
		{
			label: "missing location",
			request: &encoder.CreateStreamRequest{
				DeviceTopic:        "devices/foo",
				BrokerAddress:      "tcp://mqtt",
				DevicePrivateKey:   "privkey",
				RecipientPublicKey: "pubkey",
				UserUid:            "bob",
				Exposure:           encoder.CreateStreamRequest_INDOOR,
			},
			expectedErr: "twirp error invalid_argument: location is required",
		},
		{
			label: "missing longitude",
			request: &encoder.CreateStreamRequest{
				DeviceTopic:        "devices/foo",
				BrokerAddress:      "tcp://mqtt",
				DevicePrivateKey:   "privkey",
				RecipientPublicKey: "pubkey",
				UserUid:            "bob",
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
				DeviceTopic:        "devices/foo",
				BrokerAddress:      "tcp://mqtt",
				DevicePrivateKey:   "privkey",
				RecipientPublicKey: "pubkey",
				UserUid:            "bob",
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
			request:     &encoder.DeleteStreamRequest{},
			expectedErr: "twirp error invalid_argument: stream_uid is required",
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

	tx, err := e.db.BeginTX()
	assert.Nil(e.T(), err)

	_, err = e.db.CreateStream(tx, &postgres.Stream{
		PublicKey: "abc123",
		Device: &postgres.Device{
			Broker:    "tcp://broker:1883",
			Topic:     "devices/foo",
			UserUID:   "bob",
			Longitude: 23,
			Latitude:  45,
			Exposure:  "indoor",
		},
	})
	assert.Nil(e.T(), err)

	enc := rpc.NewEncoder(&rpc.Config{
		DB:         e.db,
		MQTTClient: mqttClient,
		Processor:  processor,
		Verbose:    true,
	}, logger)

	err = enc.(system.Startable).Start()
	assert.Nil(e.T(), err)

	err = tx.Rollback()
	assert.Nil(e.T(), err)
}

func TestRunEncoderTestSuite(t *testing.T) {
	suite.Run(t, new(EncoderTestSuite))
}
