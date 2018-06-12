package pipeline_test

import (
	"context"
	"errors"
	"testing"

	kitlog "github.com/go-kit/kit/log"
	"github.com/stretchr/testify/assert"
	datastore "github.com/thingful/twirp-datastore-go"

	"github.com/thingful/iotencoder/pkg/mocks"
	"github.com/thingful/iotencoder/pkg/pipeline"
	"github.com/thingful/iotencoder/pkg/postgres"
)

func TestProcess(t *testing.T) {
	logger := kitlog.NewNopLogger()
	ds := mocks.Datastore{}

	payload := []byte("my-data")
	encodedPayload := []byte(`73z7QQWCd4y6bOKZHW7NeA==`)

	// set up two mock responses
	ds.On(
		"WriteData",
		context.Background(),
		&datastore.WriteRequest{
			PublicKey: `BCPcL\/paSqcvCgh2Uu6LbN7o67BFY0cnqfkNUiza62fMQzDFC\/zgB5eSA\/h6nXpuSpUIg7JSOBMoxkavrL5FOj4=`,
			UserUid:   "bob",
			Data:      encodedPayload,
		},
	).Return(
		&datastore.WriteResponse{},
		nil,
	)

	ds.On(
		"WriteData",
		context.Background(),
		&datastore.WriteRequest{
			PublicKey: `BCPcL\/paSqcvCgh2Uu6LbN7o67BFY0cnqfkNUiza62fMQzDFC\/zgB5eSA\/h6nXpuSpUIg7JSOBMoxkavrL5FOj4=`,
			UserUid:   "bob",
			Data:      encodedPayload,
		},
	).Return(
		&datastore.WriteResponse{},
		nil,
	)

	processor := pipeline.NewProcessor(datastore.Datastore(&ds), true, logger)

	device := &postgres.Device{
		UserUID:    "bob",
		PrivateKey: `Dmchv5dXlMgST8ZdXOaEHmT+HvSj44nvgd0PsiSL\/FE=`,
		Streams: []*postgres.Stream{
			{
				PublicKey: `BCPcL\/paSqcvCgh2Uu6LbN7o67BFY0cnqfkNUiza62fMQzDFC\/zgB5eSA\/h6nXpuSpUIg7JSOBMoxkavrL5FOj4=`,
			},
			{
				PublicKey: `BCPcL\/paSqcvCgh2Uu6LbN7o67BFY0cnqfkNUiza62fMQzDFC\/zgB5eSA\/h6nXpuSpUIg7JSOBMoxkavrL5FOj4=`,
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

	payload := []byte("my-data")
	encodedPayload := []byte(`73z7QQWCd4y6bOKZHW7NeA==`)

	ds.On(
		"WriteData",
		context.Background(),
		&datastore.WriteRequest{
			PublicKey: `BCPcL\/paSqcvCgh2Uu6LbN7o67BFY0cnqfkNUiza62fMQzDFC\/zgB5eSA\/h6nXpuSpUIg7JSOBMoxkavrL5FOj4=`,
			UserUid:   "bob",
			Data:      encodedPayload,
		},
	).Return(
		&datastore.WriteResponse{},
		errors.New("error"),
	)

	processor := pipeline.NewProcessor(datastore.Datastore(&ds), true, logger)
	device := &postgres.Device{
		UserUID:    "bob",
		PrivateKey: `Dmchv5dXlMgST8ZdXOaEHmT+HvSj44nvgd0PsiSL\/FE=`,
		Streams: []*postgres.Stream{
			{
				PublicKey: `BCPcL\/paSqcvCgh2Uu6LbN7o67BFY0cnqfkNUiza62fMQzDFC\/zgB5eSA\/h6nXpuSpUIg7JSOBMoxkavrL5FOj4=`,
			},
		},
	}

	err := processor.Process(device, payload)
	assert.NotNil(t, err)
	assert.Equal(t, "error", err.Error())

	ds.AssertExpectations(t)
}
