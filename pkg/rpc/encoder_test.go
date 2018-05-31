package rpc_test

import (
	"context"
	"os"
	"testing"

	kitlog "github.com/go-kit/kit/log"
	"github.com/stretchr/testify/assert"
	encoder "github.com/thingful/twirp-encoder-go"

	"github.com/thingful/iotencoder/pkg/mocks"
	"github.com/thingful/iotencoder/pkg/postgres"
	"github.com/thingful/iotencoder/pkg/rpc"
	"github.com/thingful/iotencoder/pkg/system"
)

// getTestDB returns a DB instance that has been started and purged of all
// content by migrating down and up.
func getTestDB(t *testing.T, logger kitlog.Logger) postgres.DB {
	t.Helper()

	connStr := os.Getenv("IOTENCODER_DATABASE_URL")

	db := postgres.NewDB(
		&postgres.Config{
			ConnStr:            connStr,
			EncryptionPassword: "password",
			HashidSalt:         "salt",
			HashidMinLength:    8,
		},
		logger,
	)

	err := db.(system.Startable).Start()
	if err != nil {
		t.Fatalf("Failed to start DB: %v", err)
	}

	err = db.MigrateDownAll()
	if err != nil {
		t.Fatalf("Failed to migrate down: %v", err)
	}

	err = db.MigrateUp()
	if err != nil {
		t.Fatalf("Failed to migrate down: %v", err)
	}

	return db
}

func TestCreateStream(t *testing.T) {
	logger := kitlog.NewNopLogger()

	db := getTestDB(t, logger)
	defer db.(system.Stoppable).Stop()

	mqttClient := mocks.NewMQTTClient()
	processor := mocks.NewProcessor()

	enc := rpc.NewEncoder(db, mqttClient, processor, logger)
	enc.(system.Startable).Start()

	defer enc.(system.Stoppable).Stop()

	assert.NotNil(t, enc)

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
		Disposition: encoder.CreateStreamRequest_INDOOR,
	})

	assert.Nil(t, err)
	assert.Len(t, mqttClient.Subscriptions, 1)
	assert.Len(t, mqttClient.Subscriptions["tcp://mqtt.local:1883"], 1)
	assert.Equal(t, "zxkXG8ZW", resp.StreamUid)

	device, err := db.GetDevice("device/sck/abc123/readings")
	assert.Nil(t, err)
	assert.Equal(t, "tcp://mqtt.local:1883", device.Broker)
	assert.Len(t, device.Streams, 1)

	_, err = enc.DeleteStream(context.Background(), &encoder.DeleteStreamRequest{
		StreamUid: resp.StreamUid,
	})
	assert.Nil(t, err)

	device, err = db.GetDevice("device/sck/abc123/readings")
	assert.NotNil(t, err)
}

func TestSubscriptionsCreatedOnStart(t *testing.T) {
	logger := kitlog.NewNopLogger()

	db := getTestDB(t, logger)
	defer db.(system.Stoppable).Stop()

	mqttClient := mocks.NewMQTTClient()
	processor := mocks.NewProcessor()

	// insert two streams for a device
	_, err := db.CreateStream(&postgres.Stream{
		PublicKey: "abc123",
		Device: &postgres.Device{
			Broker:      "tcp://broker1:1883",
			Topic:       "devices/foo",
			UserUID:     "bob",
			Longitude:   23,
			Latitude:    23.2,
			Disposition: "indoor",
		},
	})
	assert.Nil(t, err)

	_, err = db.CreateStream(&postgres.Stream{
		PublicKey: "abc123",
		Device: &postgres.Device{
			Broker:      "tcp://broker1:1883",
			Topic:       "devices/bar",
			UserUID:     "bob",
			Longitude:   23,
			Latitude:    23.2,
			Disposition: "indoor",
		},
	})
	assert.Nil(t, err)

	enc := rpc.NewEncoder(db, mqttClient, processor, logger)
	enc.(system.Startable).Start()

	assert.Len(t, mqttClient.Subscriptions["tcp://broker1:1883"], 2)

	enc.(system.Stoppable).Stop()
}

func TestCreateStreamInvalid(t *testing.T) {
	logger := kitlog.NewNopLogger()
	db := getTestDB(t, logger)
	defer db.(system.Stoppable).Stop()

	mqttClient := mocks.NewMQTTClient()
	processor := mocks.NewProcessor()

	enc := rpc.NewEncoder(db, mqttClient, processor, logger)
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
				Disposition: encoder.CreateStreamRequest_INDOOR,
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
				Disposition: encoder.CreateStreamRequest_INDOOR,
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
				Disposition: encoder.CreateStreamRequest_INDOOR,
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
				Disposition: encoder.CreateStreamRequest_INDOOR,
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
				Disposition: encoder.CreateStreamRequest_INDOOR,
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
				Disposition:        encoder.CreateStreamRequest_INDOOR,
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
				Disposition: encoder.CreateStreamRequest_INDOOR,
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
				Disposition: encoder.CreateStreamRequest_INDOOR,
			},
			expectedErr: "twirp error invalid_argument: latitude is required",
		},
	}

	for _, tc := range testcases {
		t.Run(tc.label, func(t *testing.T) {
			_, err := enc.CreateStream(context.Background(), tc.request)
			assert.NotNil(t, err)
			assert.Equal(t, tc.expectedErr, err.Error())
		})
	}
}

func TestDeleteStreamInvalid(t *testing.T) {
	logger := kitlog.NewNopLogger()
	db := getTestDB(t, logger)
	defer db.(system.Stoppable).Stop()

	mqttClient := mocks.NewMQTTClient()
	processor := mocks.NewProcessor()

	enc := rpc.NewEncoder(db, mqttClient, processor, logger)
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
		t.Run(tc.label, func(t *testing.T) {
			_, err := enc.DeleteStream(context.Background(), tc.request)
			assert.NotNil(t, err)
			assert.Equal(t, tc.expectedErr, err.Error())
		})
	}
}

//func TestWriteDataInvalid(t *testing.T) {
//	ds := getTestDatastore(t)
//	defer ds.Stop()
//
//	testcases := []struct {
//		label         string
//		request       *datastore.WriteRequest
//		expectedError string
//	}{
//		{
//			label: "missing public_key",
//			request: &datastore.WriteRequest{
//				UserUid: "bob",
//			},
//			expectedError: "twirp error invalid_argument: public_key is required",
//		},
//		{
//			label: "missing user_uid",
//			request: &datastore.WriteRequest{
//				PublicKey: "device1",
//			},
//			expectedError: "twirp error invalid_argument: user_uid is required",
//		},
//	}
//
//	for _, tc := range testcases {
//		t.Run(tc.label, func(t *testing.T) {
//			_, err := ds.WriteData(context.Background(), tc.request)
//			assert.NotNil(t, err)
//			assert.Equal(t, tc.expectedError, err.Error())
//		})
//	}
//}
//
//func TestReadDataInvalid(t *testing.T) {
//	ds := getTestDatastore(t)
//	defer ds.Stop()
//
//	testcases := []struct {
//		label         string
//		request       *datastore.ReadRequest
//		expectedError string
//	}{
//		{
//			label:         "missing public_key",
//			request:       &datastore.ReadRequest{},
//			expectedError: "twirp error invalid_argument: public_key is required",
//		},
//		{
//			label: "large page size",
//			request: &datastore.ReadRequest{
//				PublicKey: "123abc",
//				PageSize:  1001,
//			},
//			expectedError: "twirp error invalid_argument: page_size must be between 1 and 1000",
//		},
//	}
//
//	for _, tc := range testcases {
//		t.Run(tc.label, func(t *testing.T) {
//			_, err := ds.ReadData(context.Background(), tc.request)
//			assert.NotNil(t, err)
//			assert.Equal(t, tc.expectedError, err.Error())
//		})
//	}
//}
//
//func TestDeleteDataInvalid(t *testing.T) {
//	ds := getTestDatastore(t)
//	defer ds.Stop()
//
//	testcases := []struct {
//		label         string
//		request       *datastore.DeleteRequest
//		expectedError string
//	}{
//		{
//			label:         "missing user_uid",
//			request:       &datastore.DeleteRequest{},
//			expectedError: "twirp error invalid_argument: user_uid is required",
//		},
//	}
//
//	for _, tc := range testcases {
//		t.Run(tc.label, func(t *testing.T) {
//			_, err := ds.DeleteData(context.Background(), tc.request)
//			assert.NotNil(t, err)
//			assert.Equal(t, tc.expectedError, err.Error())
//		})
//	}
//}
//
//func TestPagination(t *testing.T) {
//	ds := getTestDatastore(t)
//	defer ds.Stop()
//
//	fixtures := []struct {
//		publicKey string
//		userID    string
//		timestamp string
//		data      []byte
//	}{
//		{
//			publicKey: "abc123",
//			userID:    "alice",
//			timestamp: "2018-05-01T08:00:00Z",
//			data:      []byte("first"),
//		},
//		{
//			publicKey: "abc123",
//			userID:    "alice",
//			timestamp: "2018-05-01T08:02:00Z",
//			data:      []byte("third"),
//		},
//		{
//			publicKey: "abc123",
//			userID:    "bob",
//			timestamp: "2018-05-01T08:01:00Z",
//			data:      []byte("second"),
//		},
//		{
//			publicKey: "abc123",
//			userID:    "bob",
//			timestamp: "2018-05-01T08:02:00Z",
//			data:      []byte("fourth"),
//		},
//	}
//
//	// load fixtures into db
//	for _, f := range fixtures {
//		ts, _ := time.Parse(time.RFC3339, f.timestamp)
//
//		ds.DB.MustExec("INSERT INTO events (public_key, user_uid, recorded_at, data) VALUES ($1, $2, $3, $4)", f.publicKey, f.userID, ts, f.data)
//	}
//
//	resp, err := ds.ReadData(context.Background(), &datastore.ReadRequest{
//		PublicKey: "abc123",
//		PageSize:  3,
//	})
//
//	assert.Nil(t, err)
//	assert.Equal(t, "abc123", resp.PublicKey)
//	assert.Len(t, resp.Events, 3)
//	assert.NotEqual(t, "", resp.NextPageCursor)
//
//	assert.Equal(t, "first", string(resp.Events[0].Data))
//	assert.Equal(t, "second", string(resp.Events[1].Data))
//	assert.Equal(t, "third", string(resp.Events[2].Data))
//
//	resp, err = ds.ReadData(context.Background(), &datastore.ReadRequest{
//		PublicKey:  "abc123",
//		PageSize:   3,
//		PageCursor: resp.NextPageCursor,
//	})
//
//	assert.Nil(t, err)
//	assert.Len(t, resp.Events, 1)
//	assert.Equal(t, "", resp.NextPageCursor)
//}
//
