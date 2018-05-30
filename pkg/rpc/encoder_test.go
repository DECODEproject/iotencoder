package rpc_test

import (
	"context"
	"os"
	"testing"

	kitlog "github.com/go-kit/kit/log"
	"github.com/stretchr/testify/assert"

	"github.com/thingful/iotencoder/pkg/mocks"
	"github.com/thingful/iotencoder/pkg/postgres"
	"github.com/thingful/iotencoder/pkg/rpc"
	"github.com/thingful/iotencoder/pkg/system"
	encoder "github.com/thingful/twirp-encoder-go"
)

// getTestEncoder is a helper function that returns an encoder, and also does
// some housekeeping to clean the DB by rolling back and reapplying migrations.
//
// TODO: not terribly happy with this as an approach. See if we can think of an
// alternative.
func getTestEncoder(t *testing.T) (*rpc.Encoder, *mocks.MQTTClient, postgres.DB) {
	t.Helper()

	logger := kitlog.NewNopLogger()
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

	m := mocks.NewMQTTClient()

	// create encoder
	enc := rpc.NewEncoder(logger, m, db)

	err := db.(system.Component).Start()
	if err != nil {
		t.Fatalf("Error starting DB: %v", err)
	}

	err = db.MigrateDownAll()
	if err != nil {
		t.Fatalf("Error migrating DB down: %v", err)
	}

	err = db.MigrateUp()
	if err != nil {
		t.Fatalf("Error migrating DB up: %v", err)
	}

	err = enc.Start()
	if err != nil {
		t.Fatalf("Error starting encoder: %v", err)
	}

	return enc, m, db
}

func TestCreateStream(t *testing.T) {
	enc, m, db := getTestEncoder(t)
	defer enc.Stop()

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
	assert.Len(t, m.Subscriptions, 1)
	assert.Equal(t, "zxkXG8ZW", resp.StreamUid)

	device, err := db.GetDevice("device/sck/abc123/readings")
	assert.Nil(t, err)
	assert.Equal(t, "tcp://mqtt.local:1883", device.Broker)
	assert.Len(t, device.Streams, 1)

	_, err = enc.DeleteStream(context.Background(), &encoder.DeleteStreamRequest{
		StreamUid: resp.StreamUid,
	})
	assert.Nil(t, err)
}

//func TestRoundTrip(t *testing.T) {
//	ds := getTestDatastore(t)
//	defer ds.Stop()
//
//	_, err := ds.WriteData(context.Background(), &datastore.WriteRequest{
//		PublicKey: "123abc",
//		UserUid:   "bob",
//		Data:      []byte("hello world"),
//	})
//	assert.Nil(t, err)
//
//	var count int
//	err = ds.DB.Get(&count, ds.DB.Rebind("SELECT COUNT(*) FROM events WHERE public_key = ?"), "123abc")
//	assert.Nil(t, err)
//	assert.Equal(t, 1, count)
//
//	resp, err := ds.ReadData(context.Background(), &datastore.ReadRequest{
//		PublicKey: "123abc",
//	})
//	assert.Nil(t, err)
//	assert.Equal(t, "123abc", resp.PublicKey)
//	assert.Len(t, resp.Events, 1)
//	assert.Equal(t, int(rpc.DefaultPageSize), int(resp.PageSize))
//	assert.Equal(t, "", resp.NextPageCursor)
//
//	event := resp.Events[0]
//	assert.Equal(t, []byte("hello world"), event.Data)
//
//	_, err = ds.DeleteData(context.Background(), &datastore.DeleteRequest{
//		UserUid: "bob",
//	})
//	assert.Nil(t, err)
//
//	resp, err = ds.ReadData(context.Background(), &datastore.ReadRequest{
//		PublicKey: "123abc",
//	})
//	assert.Nil(t, err)
//	assert.Equal(t, "123abc", resp.PublicKey)
//	assert.Len(t, resp.Events, 0)
//}

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
