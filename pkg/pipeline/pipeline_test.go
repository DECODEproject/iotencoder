package pipeline_test

import (
	"context"
	"testing"

	kitlog "github.com/go-kit/kit/log"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	datastore "github.com/thingful/twirp-datastore-go"

	"github.com/DECODEproject/iotencoder/pkg/mocks"
	"github.com/DECODEproject/iotencoder/pkg/pipeline"
	"github.com/DECODEproject/iotencoder/pkg/postgres"
)

// TODO: Remind yourself what this Process function is supposed to do and
// therefore why these tests are failing

func TestProcess(t *testing.T) {
	logger := kitlog.NewNopLogger()
	ds := mocks.Datastore{}

	payload := []byte(`{"data":[{"recorded_at":"2018-12-11T14:46:44Z","sensors":[{"id":13, "value":51.00},{"id":14, "value":426.42},{"id":12, "value":12.58},{"id":29, "value":79.35},{"id":53, "value":51.00},{"id":58, "value":101.56},{"id":89, "value":4.00},{"id":87, "value":7.00},{"id":88, "value":7.00}]}]}`)

	// set up a mock response
	ds.On(
		"WriteData",
		context.Background(),
		mock.Anything,
	).Return(
		&datastore.WriteResponse{},
		nil,
	)

	processor := pipeline.NewProcessor(datastore.Datastore(&ds), true, logger)

	device := &postgres.Device{
		DeviceToken: "foo",
		Streams: []*postgres.Stream{
			{
				PolicyID:  "smartcitizen",
				PublicKey: `BBLewg4VqLR38b38daE7Fj\/uhr543uGrEpyoPFgmFZK6EZ9g2XdK\/i65RrSJ6sJ96aXD3DJHY3Me2GJQO9\/ifjE=`,
				Operations: postgres.Operations{
					&postgres.Operation{
						SensorID: 13,
						Action:   pipeline.Share,
					},
					&postgres.Operation{
						SensorID: 14,
						Action:   pipeline.Share,
					},
					&postgres.Operation{
						SensorID: 12,
						Action:   pipeline.Share,
					},
					&postgres.Operation{
						SensorID: 29,
						Action:   pipeline.Bin,
						Bins:     []float64{30, 80, 120},
					},
				},
			},
		},
	}

	err := processor.Process(device, payload)
	assert.Nil(t, err)

	ds.AssertExpectations(t)
}

func TestProcessWithError(t *testing.T) {
	logger := kitlog.NewNopLogger()
	ds := mocks.Datastore{}

	payload := []byte(`{"data":[{"recorded_at":"2018-12-11T14:46:44Z","sensors":[{"id":13, "value":51.00},{"id":14, "value":426.42},{"id":12, "value":12.58},{"id":29, "value":79.35},{"id":53, "value":51.00},{"id":58, "value":101.56},{"id":89, "value":4.00},{"id":87, "value":7.00},{"id":88, "value":7.00}]}]}`)

	ds.On(
		"WriteData",
		context.Background(),
		mock.Anything,
	).Return(
		&datastore.WriteResponse{},
		errors.New("error"),
	)

	processor := pipeline.NewProcessor(datastore.Datastore(&ds), true, logger)
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
