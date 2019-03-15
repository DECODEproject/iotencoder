package rpc_test

import (
	"context"
	"errors"
	"os"
	"testing"

	kitlog "github.com/go-kit/kit/log"
	"github.com/google/uuid"
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
		DB:             e.db,
		MQTTClient:     mqttClient,
		Processor:      processor,
		Verbose:        false,
		BrokerAddr:     "tcp://mqtt.local:1883",
		BrokerUsername: "decode",
	}, logger)

	assert.Len(e.T(), mqttClient.Subscriptions, 0)

	err := enc.(system.Startable).Start()
	assert.Nil(e.T(), err)
	defer enc.(system.Stoppable).Stop()

	resp, err := enc.CreateStream(context.Background(), &encoder.CreateStreamRequest{
		DeviceToken:        "abc123",
		RecipientPublicKey: "pub_key",
		CommunityId:        "policy-id",
		Location: &encoder.CreateStreamRequest_Location{
			Longitude: -0.024,
			Latitude:  54.24,
		},
		Exposure: encoder.CreateStreamRequest_INDOOR,
	})
	assert.Nil(e.T(), err)

	assert.Len(e.T(), mqttClient.Subscriptions, 1)
	assert.Len(e.T(), mqttClient.Subscriptions["tcp://mqtt.local:1883:decode"], 1)
	assert.NotEqual(e.T(), "", resp.StreamUid)

	device, err := e.db.GetDevice("abc123")
	assert.Nil(e.T(), err)
	assert.Len(e.T(), device.Streams, 1)

	_, err = enc.DeleteStream(context.Background(), &encoder.DeleteStreamRequest{
		StreamUid: resp.StreamUid,
		Token:     resp.Token,
	})
	assert.Nil(e.T(), err)

	device, err = e.db.GetDevice("abc123")
	assert.NotNil(e.T(), err)
}

func (e *EncoderTestSuite) TestStreamWithOperationsLifecycle() {
	logger := kitlog.NewNopLogger()
	mqttClient := mocks.NewMQTTClient(nil)
	processor := mocks.NewProcessor()

	enc := rpc.NewEncoder(&rpc.Config{
		DB:             e.db,
		MQTTClient:     mqttClient,
		Processor:      processor,
		Verbose:        false,
		BrokerAddr:     "tcp://mqtt.local:1883",
		BrokerUsername: "decode",
	}, logger)

	assert.Len(e.T(), mqttClient.Subscriptions, 0)

	err := enc.(system.Startable).Start()
	assert.Nil(e.T(), err)
	defer enc.(system.Stoppable).Stop()

	resp, err := enc.CreateStream(context.Background(), &encoder.CreateStreamRequest{
		DeviceToken:        "abc123",
		RecipientPublicKey: "pub_key",
		CommunityId:        "policy-id",
		Location: &encoder.CreateStreamRequest_Location{
			Longitude: -0.024,
			Latitude:  54.24,
		},
		Exposure: encoder.CreateStreamRequest_INDOOR,
		Operations: []*encoder.CreateStreamRequest_Operation{
			&encoder.CreateStreamRequest_Operation{
				SensorId: 13,
				Action:   encoder.CreateStreamRequest_Operation_SHARE,
			},
			&encoder.CreateStreamRequest_Operation{
				SensorId: 14,
				Action:   encoder.CreateStreamRequest_Operation_BIN,
				Bins:     []float64{5.0, 10.0},
			},
			&encoder.CreateStreamRequest_Operation{
				SensorId: 16,
				Action:   encoder.CreateStreamRequest_Operation_MOVING_AVG,
				Interval: 900,
			},
		},
	})
	assert.Nil(e.T(), err)

	assert.Len(e.T(), mqttClient.Subscriptions, 1)
	assert.Len(e.T(), mqttClient.Subscriptions["tcp://mqtt.local:1883:decode"], 1)
	assert.NotEqual(e.T(), "", resp.StreamUid)

	device, err := e.db.GetDevice("abc123")
	assert.Nil(e.T(), err)
	assert.Len(e.T(), device.Streams, 1)

	stream := device.Streams[0]
	assert.Len(e.T(), stream.Operations, 3)

	assert.Equal(e.T(), 13, int(stream.Operations[0].SensorID))
	assert.Equal(e.T(), postgres.Action("SHARE"), stream.Operations[0].Action)

	assert.Equal(e.T(), 14, int(stream.Operations[1].SensorID))
	assert.Equal(e.T(), postgres.Action("BIN"), stream.Operations[1].Action)
	assert.Equal(e.T(), []float64{5.0, 10.0}, stream.Operations[1].Bins)

	assert.Equal(e.T(), 16, int(stream.Operations[2].SensorID))
	assert.Equal(e.T(), postgres.Action("MOVING_AVG"), stream.Operations[2].Action)
	assert.Equal(e.T(), 900, int(stream.Operations[2].Interval))

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
		PublicKey:   "abc123",
		CommunityID: "policy-id",
		Device: &postgres.Device{
			DeviceToken: "foo",
			Longitude:   23,
			Latitude:    23.2,
			Exposure:    "indoor",
		},
	})
	assert.Nil(e.T(), err)

	_, err = e.db.CreateStream(&postgres.Stream{
		PublicKey:   "abc123",
		CommunityID: "policy-id-2",
		Device: &postgres.Device{
			DeviceToken: "bar",
			Longitude:   23,
			Latitude:    23.2,
			Exposure:    "indoor",
		},
	})
	assert.Nil(e.T(), err)

	enc := rpc.NewEncoder(&rpc.Config{
		DB:             e.db,
		MQTTClient:     mqttClient,
		Processor:      processor,
		Verbose:        true,
		BrokerAddr:     "tcp://broker1:1883",
		BrokerUsername: "decode",
	}, logger)

	enc.(system.Startable).Start()

	assert.Len(e.T(), mqttClient.Subscriptions["tcp://broker1:1883:decode"], 2)

	enc.(system.Stoppable).Stop()
}

func (e *EncoderTestSuite) TestCreateStreamInvalid() {
	logger := kitlog.NewNopLogger()
	mqttClient := mocks.NewMQTTClient(nil)
	processor := mocks.NewProcessor()

	enc := rpc.NewEncoder(&rpc.Config{
		DB:             e.db,
		MQTTClient:     mqttClient,
		Processor:      processor,
		Verbose:        true,
		BrokerAddr:     "tcp://mqtt",
		BrokerUsername: "decode",
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
			expectedErr: "twirp error invalid_argument: community_id is required",
		},
		{
			label: "missing public key",
			request: &encoder.CreateStreamRequest{
				DeviceToken: "foo",
				CommunityId: "policy-id",
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
				CommunityId:        "policy-id",
				RecipientPublicKey: "pubkey",
				Exposure:           encoder.CreateStreamRequest_INDOOR,
			},
			expectedErr: "twirp error invalid_argument: location is required",
		},
		{
			label: "missing longitude",
			request: &encoder.CreateStreamRequest{
				DeviceToken:        "foo",
				CommunityId:        "policy-id",
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
				CommunityId:        "policy-id",
				RecipientPublicKey: "pubkey",
				Location: &encoder.CreateStreamRequest_Location{
					Longitude: 45,
				},
				Exposure: encoder.CreateStreamRequest_INDOOR,
			},
			expectedErr: "twirp error invalid_argument: latitude is required",
		},
		{
			label: "operation with no sensor id",
			request: &encoder.CreateStreamRequest{
				DeviceToken:        "abc123",
				RecipientPublicKey: "pub_key",
				CommunityId:        "policy-id",
				Location: &encoder.CreateStreamRequest_Location{
					Longitude: -0.024,
					Latitude:  54.24,
				},
				Exposure: encoder.CreateStreamRequest_INDOOR,
				Operations: []*encoder.CreateStreamRequest_Operation{
					&encoder.CreateStreamRequest_Operation{
						Action: encoder.CreateStreamRequest_Operation_SHARE,
					},
				},
			},
			expectedErr: "twirp error invalid_argument: operations require a non-zero sensor id",
		},
		{
			label: "bin with no bins",
			request: &encoder.CreateStreamRequest{
				DeviceToken:        "abc123",
				RecipientPublicKey: "pub_key",
				CommunityId:        "policy-id",
				Location: &encoder.CreateStreamRequest_Location{
					Longitude: -0.024,
					Latitude:  54.24,
				},
				Exposure: encoder.CreateStreamRequest_INDOOR,
				Operations: []*encoder.CreateStreamRequest_Operation{
					&encoder.CreateStreamRequest_Operation{
						SensorId: 13,
						Action:   encoder.CreateStreamRequest_Operation_BIN,
					},
				},
			},
			expectedErr: "twirp error invalid_argument: operations binning requires a non-empty list of bins",
		},
		{
			label: "moving average no interval",
			request: &encoder.CreateStreamRequest{
				DeviceToken:        "abc123",
				RecipientPublicKey: "pub_key",
				CommunityId:        "policy-id",
				Location: &encoder.CreateStreamRequest_Location{
					Longitude: -0.024,
					Latitude:  54.24,
				},
				Exposure: encoder.CreateStreamRequest_INDOOR,
				Operations: []*encoder.CreateStreamRequest_Operation{
					&encoder.CreateStreamRequest_Operation{
						SensorId: 13,
						Action:   encoder.CreateStreamRequest_Operation_MOVING_AVG,
					},
				},
			},
			expectedErr: "twirp error invalid_argument: operations moving average requires a non-zero interval",
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
		DB:             e.db,
		MQTTClient:     mqttClient,
		Processor:      processor,
		Verbose:        true,
		BrokerAddr:     "tcp://mqtt:1883",
		BrokerUsername: "decode",
	}, logger)

	enc.(system.Startable).Start()
	defer enc.(system.Stoppable).Stop()

	streamID := uuid.New().String()

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
			request:     &encoder.DeleteStreamRequest{StreamUid: streamID},
			expectedErr: "twirp error invalid_argument: token is required",
		},
		{
			label:       "missing stream",
			request:     &encoder.DeleteStreamRequest{StreamUid: streamID, Token: "barfoo"},
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
		PublicKey:   "abc123",
		CommunityID: "policy-id",
		Device: &postgres.Device{
			DeviceToken: "foo",
			Longitude:   23,
			Latitude:    45,
			Exposure:    "indoor",
		},
	})
	assert.Nil(e.T(), err)

	enc := rpc.NewEncoder(&rpc.Config{
		DB:             e.db,
		MQTTClient:     mqttClient,
		Processor:      processor,
		Verbose:        true,
		BrokerAddr:     "tcp://broker:1883",
		BrokerUsername: "decode",
	}, logger)

	err = enc.(system.Startable).Start()
	assert.Nil(e.T(), err)
}

func TestRunEncoderTestSuite(t *testing.T) {
	suite.Run(t, new(EncoderTestSuite))
}
