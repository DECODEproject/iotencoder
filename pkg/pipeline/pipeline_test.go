package pipeline_test

import (
	"context"
	"testing"

	kitlog "github.com/go-kit/kit/log"
	"github.com/stretchr/testify/assert"
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

	payload := []byte(`{"msg": "data"}`)
	//encodedPayload := []byte(`{"header":"gqxjb21tdW5pdHlfaWSsc21hcnRjaXRpemVurWRldmljZV9wdWJrZXnZWEJBVThBVUxRdTZlZzZzU0NCZUlMS1VsbVo3eTNNTDhDQUVlTE9QcTA3NEd1WE5IUU16MVBsNS8xaW1HVkJSTWY3WTlqZkI2cW5ENzlreWMvOHcxY283WT0=","iv":"5qF2FtTcB\/gqo0wg8xs6CA==","checksum":"dY\/m9Jc0ZDLqs5e7ZZrU3w==","encoding":"base64","curve":"ed25519","zenroom":"0.8.1","text":"KT7x2qYJZhaIPQ=="}`)
	encodedPayload := []byte(`{"device_token":"foo","community_id":"smartcitizen","community_pubkey":"BBLewg4VqLR38b38daE7Fj\/uhr543uGrEpyoPFgmFZK6EZ9g2XdK\/i65RrSJ6sJ96aXD3DJHY3Me2GJQO9\/ifjE="}`)

	// set up a mock response
	ds.On(
		"WriteData",
		context.Background(),
		&datastore.WriteRequest{
			PublicKey:   `BBLewg4VqLR38b38daE7Fj\/uhr543uGrEpyoPFgmFZK6EZ9g2XdK\/i65RrSJ6sJ96aXD3DJHY3Me2GJQO9\/ifjE=`,
			DeviceToken: "foo",
			Data:        encodedPayload,
		},
	).Return(
		&datastore.WriteResponse{},
		nil,
	)

	//ds.On(
	//	"WriteData",
	//	context.Background(),
	//	&datastore.WriteRequest{
	//		PublicKey:   `BCPcL\/paSqcvCgh2Uu6LbN7o67BFY0cnqfkNUiza62fMQzDFC\/zgB5eSA\/h6nXpuSpUIg7JSOBMoxkavrL5FOj4=`,
	//		DeviceToken: "bar",
	//		Data:        encodedPayload,
	//	},
	//).Return(
	//	&datastore.WriteResponse{},
	//	nil,
	//)

	processor := pipeline.NewProcessor(datastore.Datastore(&ds), true, logger)

	device := &postgres.Device{
		DeviceToken: "foo",
		Streams: []*postgres.Stream{
			{
				PolicyID:  "smartcitizen",
				PublicKey: `BBLewg4VqLR38b38daE7Fj\/uhr543uGrEpyoPFgmFZK6EZ9g2XdK\/i65RrSJ6sJ96aXD3DJHY3Me2GJQO9\/ifjE=`,
			},
			//{
			//	PublicKey: `BCPcL\/paSqcvCgh2Uu6LbN7o67BFY0cnqfkNUiza62fMQzDFC\/zgB5eSA\/h6nXpuSpUIg7JSOBMoxkavrL5FOj4=`,
			//},
		},
	}

	err := processor.Process(device, payload)
	assert.Nil(t, err)

	ds.AssertExpectations(t)
}

/*func TestProcessWithError(t *testing.T) {
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
*/
