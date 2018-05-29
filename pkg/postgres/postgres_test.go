package postgres_test

import (
	"os"
	"testing"

	kitlog "github.com/go-kit/kit/log"
	"github.com/stretchr/testify/assert"

	"github.com/thingful/iotencoder/pkg/postgres"
	"github.com/thingful/iotencoder/pkg/system"
)

func getTestingDB(t *testing.T) postgres.DB {
	t.Helper()

	logger := kitlog.NewNopLogger()
	connStr := os.Getenv("IOTENCODER_DATABASE_URL")
	encryptionPassword := os.Getenv("IOTENCODER_ENCRYPTION_PASSWORD")

	db := postgres.NewDB(connStr, encryptionPassword, logger)
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

	return db
}
func TestRoundTrip(t *testing.T) {
	db := getTestingDB(t)
	defer db.(system.Component).Stop()

	assert.NotNil(t, db)

	streamID, err := db.CreateStream(&postgres.Stream{
		PublicKey: "public",
		Device: &postgres.Device{
			Broker:      "tcp://example.com",
			Topic:       "device/123",
			PrivateKey:  "private",
			UserUID:     "bob",
			Longitude:   45.2,
			Latitude:    23.2,
			Disposition: "indoor",
		},
	})

	assert.Nil(t, err)

	devices, err := db.GetDevices()
	assert.Nil(t, err)
	assert.Len(t, devices, 1)

	assert.Equal(t, "tcp://example.com", devices[0].Broker)
	assert.Equal(t, "device/123", devices[0].Topic)
	assert.Equal(t, "private", devices[0].PrivateKey)

	device, err := db.GetDevice("device/123")
	assert.Nil(t, err)

	assert.Equal(t, "tcp://example.com", device.Broker)
	assert.Equal(t, "device/123", device.Topic)
	assert.Equal(t, "private", device.PrivateKey)
	assert.Equal(t, "bob", device.UserUID)
	assert.Equal(t, 45.2, device.Longitude)
	assert.Equal(t, 23.2, device.Latitude)
	assert.Equal(t, "indoor", device.Disposition)
	assert.Len(t, device.Streams, 1)
	assert.Equal(t, "public", device.Streams[0].PublicKey)

	err = db.DeleteStream(streamID)
	assert.Nil(t, err)

	devices, err = db.GetDevices()
	assert.Nil(t, err)
	assert.Len(t, devices, 0)
}
