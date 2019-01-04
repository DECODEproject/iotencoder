package pipeline_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/DECODEproject/zenroom-go"
	kitlog "github.com/go-kit/kit/log"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	datastore "github.com/thingful/twirp-datastore-go"

	"github.com/DECODEproject/iotencoder/pkg/lua"
	"github.com/DECODEproject/iotencoder/pkg/mocks"
	"github.com/DECODEproject/iotencoder/pkg/pipeline"
	"github.com/DECODEproject/iotencoder/pkg/postgres"
	"github.com/DECODEproject/iotencoder/pkg/smartcitizen"
)

func decryptData(t *testing.T, call mock.Call, secKey string) (*smartcitizen.Device, error) {
	req := call.Arguments[1].(*datastore.WriteRequest)

	decryptKeys := []byte(fmt.Sprintf(`{"community_seckey":"%s"}`, secKey))

	decryptScript, err := lua.Asset("decrypt.lua")
	assert.Nil(t, err)

	output, err := zenroom.Exec(
		decryptScript,
		zenroom.WithKeys(decryptKeys),
		zenroom.WithData(req.Data),
		zenroom.WithVerbosity(1),
	)
	assert.Nil(t, err)
	assert.NotNil(t, output)

	var unmarshalled map[string]interface{}
	err = json.Unmarshal(output, &unmarshalled)
	assert.Nil(t, err)

	var decryptedDevice smartcitizen.Device
	err = json.Unmarshal([]byte(unmarshalled["data"].(string)), &decryptedDevice)
	assert.Nil(t, err)

	return &decryptedDevice, nil
}

func TestProcess(t *testing.T) {
	logger := kitlog.NewNopLogger()
	ds := mocks.Datastore{}

	// set up a mock response
	ds.On(
		"WriteData",
		context.Background(),
		mock.Anything,
	).Return(
		&datastore.WriteResponse{},
		nil,
	)

	rd := mocks.Redis{}
	rd.On(
		"MovingAverage",
		12.58,
		"foo",
		12,
		uint32(900),
	).Return(
		12.58,
		nil,
	)

	payload := []byte(`{"data":[{"recorded_at":"2018-12-11T14:46:44Z","sensors":[{"id":13, "value":51.00},{"id":14, "value":426.42},{"id":12, "value":12.58},{"id":29, "value":79.35},{"id":53, "value":51.00},{"id":58, "value":101.56},{"id":89, "value":4.00},{"id":87, "value":7.00},{"id":88, "value":7.00}]}]}`)

	processor := pipeline.NewProcessor(datastore.Datastore(&ds), &rd, true, logger)

	device := &postgres.Device{
		DeviceToken: "foo",
		Streams: []*postgres.Stream{
			{
				PolicyID:  "smartcitizen",
				PublicKey: `BBLewg4VqLR38b38daE7Fj\/uhr543uGrEpyoPFgmFZK6EZ9g2XdK\/i65RrSJ6sJ96aXD3DJHY3Me2GJQO9\/ifjE=`,
				Operations: postgres.Operations{
					&postgres.Operation{
						SensorID: 13,
						Action:   postgres.Share,
					},
					&postgres.Operation{
						SensorID: 14,
						Action:   postgres.Share,
					},
					&postgres.Operation{
						SensorID: 12,
						Action:   postgres.MovingAverage,
						Interval: 900,
					},
					&postgres.Operation{
						SensorID: 29,
						Action:   postgres.Bin,
						Bins:     []float64{30, 80, 120},
					},
				},
			},
		},
	}

	err := processor.Process(device, payload)
	assert.Nil(t, err)

	ds.AssertExpectations(t)
	rd.AssertExpectations(t)

	assert.Len(t, ds.Calls, 1)

	decryptedDevice, err := decryptData(t, ds.Calls[0], "D19GsDTGjLBX23J281SNpXWUdu+oL6hdAJ0Zh6IrRHA=")
	assert.Nil(t, err)

	assert.Len(t, decryptedDevice.Sensors, 4)
}

func TestProcessWithNoOperations(t *testing.T) {
	logger := kitlog.NewNopLogger()
	ds := mocks.Datastore{}

	// set up a mock response
	ds.On(
		"WriteData",
		context.Background(),
		mock.Anything,
	).Return(
		&datastore.WriteResponse{},
		nil,
	)

	rd := mocks.Redis{}

	payload := []byte(`{"data":[{"recorded_at":"2018-12-11T14:46:44Z","sensors":[{"id":13, "value":51.00},{"id":14, "value":426.42},{"id":12, "value":12.58},{"id":29, "value":79.35},{"id":53, "value":51.00},{"id":58, "value":101.56},{"id":89, "value":4.00},{"id":87, "value":7.00},{"id":88, "value":7.00}]}]}`)

	processor := pipeline.NewProcessor(datastore.Datastore(&ds), &rd, true, logger)

	device := &postgres.Device{
		DeviceToken: "foo",
		Streams: []*postgres.Stream{
			{
				PolicyID:   "smartcitizen",
				PublicKey:  `BBLewg4VqLR38b38daE7Fj\/uhr543uGrEpyoPFgmFZK6EZ9g2XdK\/i65RrSJ6sJ96aXD3DJHY3Me2GJQO9\/ifjE=`,
				Operations: postgres.Operations{},
			},
		},
	}

	err := processor.Process(device, payload)
	assert.Nil(t, err)

	ds.AssertExpectations(t)
	rd.AssertExpectations(t)

	assert.Len(t, ds.Calls, 1)

	decryptedDevice, err := decryptData(t, ds.Calls[0], "D19GsDTGjLBX23J281SNpXWUdu+oL6hdAJ0Zh6IrRHA=")
	assert.Nil(t, err)

	assert.Len(t, decryptedDevice.Sensors, 9)
}

func TestProcessWithDatastoreError(t *testing.T) {
	logger := kitlog.NewNopLogger()
	ds := mocks.Datastore{}

	ds.On(
		"WriteData",
		context.Background(),
		mock.Anything,
	).Return(
		&datastore.WriteResponse{},
		errors.New("error"),
	)

	rd := mocks.Redis{}
	rd.On(
		"MovingAverage",
		12.58,
		"foo",
		12,
		uint32(900),
	).Return(
		12.58,
		nil,
	)

	payload := []byte(`{"data":[{"recorded_at":"2018-12-11T14:46:44Z","sensors":[{"id":13, "value":51.00},{"id":14, "value":426.42},{"id":12, "value":12.58},{"id":29, "value":79.35},{"id":53, "value":51.00},{"id":58, "value":101.56},{"id":89, "value":4.00},{"id":87, "value":7.00},{"id":88, "value":7.00}]}]}`)

	processor := pipeline.NewProcessor(&ds, &rd, true, logger)
	device := &postgres.Device{
		DeviceToken: "foo",
		Streams: []*postgres.Stream{
			{
				PolicyID:  "smartcitizen",
				PublicKey: `BBLewg4VqLR38b38daE7Fj\/uhr543uGrEpyoPFgmFZK6EZ9g2XdK\/i65RrSJ6sJ96aXD3DJHY3Me2GJQO9\/ifjE=`,
			},
		},
	}

	err := processor.Process(device, payload)
	assert.NotNil(t, err)
	assert.Equal(t, "error", err.Error())

	ds.AssertExpectations(t)
}

func TestProcessWithRedisError(t *testing.T) {
	logger := kitlog.NewNopLogger()
	ds := mocks.Datastore{}

	ds.On(
		"WriteData",
		context.Background(),
		mock.Anything,
	).Return(
		&datastore.WriteResponse{},
		nil,
	)

	rd := mocks.Redis{}
	rd.On(
		"MovingAverage",
		12.58,
		"foo",
		12,
		uint32(900),
	).Return(
		0.0,
		errors.New("error"),
	)

	payload := []byte(`{"data":[{"recorded_at":"2018-12-11T14:46:44Z","sensors":[{"id":13, "value":51.00},{"id":14, "value":426.42},{"id":12, "value":12.58},{"id":29, "value":79.35},{"id":53, "value":51.00},{"id":58, "value":101.56},{"id":89, "value":4.00},{"id":87, "value":7.00},{"id":88, "value":7.00}]}]}`)

	processor := pipeline.NewProcessor(&ds, &rd, true, logger)
	device := &postgres.Device{
		DeviceToken: "foo",
		Streams: []*postgres.Stream{
			{
				PolicyID:  "smartcitizen",
				PublicKey: `BBLewg4VqLR38b38daE7Fj\/uhr543uGrEpyoPFgmFZK6EZ9g2XdK\/i65RrSJ6sJ96aXD3DJHY3Me2GJQO9\/ifjE=`,
				Operations: postgres.Operations{
					&postgres.Operation{
						SensorID: 13,
						Action:   postgres.Share,
					},
					&postgres.Operation{
						SensorID: 14,
						Action:   postgres.Share,
					},
					&postgres.Operation{
						SensorID: 12,
						Action:   postgres.MovingAverage,
						Interval: 900,
					},
					&postgres.Operation{
						SensorID: 29,
						Action:   postgres.Bin,
						Bins:     []float64{30, 80, 120},
					},
				},
			},
		},
	}

	err := processor.Process(device, payload)
	assert.NotNil(t, err)
	assert.Equal(t, "failed to calculate moving average: error", err.Error())

	//ds.AssertExpectations(t)
}
