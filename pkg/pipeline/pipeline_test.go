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
	payload := []byte(`{"data":[{"recorded_at":"2018-06-03T22:26:02Z","sensors":[{"id":10,"value":100},{"id":29,"value":67.02939},{"id":12,"value":22.41268}]}]}`)

	testcases := []struct {
		label             string
		device            *postgres.Device
		encryptedPayloads [][]byte
	}{
		{
			label: "no entitlements",
			device: &postgres.Device{
				UserUID:    "bob",
				PrivateKey: `Dmchv5dXlMgST8ZdXOaEHmT+HvSj44nvgd0PsiSL\/FE=`,
				Longitude:  2.5,
				Latitude:   2.5,
				Exposure:   "INDOOR",
				Streams: []*postgres.Stream{
					{
						PublicKey: `BCPcL\/paSqcvCgh2Uu6LbN7o67BFY0cnqfkNUiza62fMQzDFC\/zgB5eSA\/h6nXpuSpUIg7JSOBMoxkavrL5FOj4=`,
					},
				},
			},
			encryptedPayloads: [][]byte{
				[]byte(`B5yRzPqlmh+4rmSnxNMJ1W8NBv8Om0TkvWraCx27u1H+yRL+P+DTZ6+YjoSKzvHeWEOStuJt7XnFY8H2QiTiLwjkmRoxBvzqCZ2ffbz7zAU7di8PzrRtRF/uRx0LgLrtzbfbJTgIiRvCr0D3x+nJIHIsI9TzEI0rAqT6uNcNnkNBmypcsFssRbwg1yRg4FIl53iIQ3A8c23B74NYy+24PRHidE+ncW7T2FxjhC8a4PB8UEaDcB0cmT4RJM+OX2hrzCBizv8mktAwbJKH5aHai92SybqV49sIFO48xJqnlINNHZh/pZZURFrS1lp4bKEaKOuh5G0mkxtvgADkhB0BVCKwnaFNpiy4NYddGqQZRcam7CyIklF81obOyWAgc+abxfDIjjdt6GVDznQGpIRr3QMnx4E+qfvsc2R/+DunG+ECrKo1FwU+MXHV6N3Foql9zRjijp/uXJiP6Vsiy3GgZgBC8ZzsgKVxYAM08WEyFD0gT+p/uqBhqzBI8EXsaxdQgEHmhvenBX7er6EJ3r30Ye9tau/4X/vMLYGLvim4hTA=`),
			},
		},
		{
			label: "share entitlement",
			device: &postgres.Device{
				UserUID:    "bob",
				PrivateKey: `Dmchv5dXlMgST8ZdXOaEHmT+HvSj44nvgd0PsiSL\/FE=`,
				Longitude:  2.5,
				Latitude:   2.5,
				Exposure:   "INDOOR",
				Streams: []*postgres.Stream{
					&postgres.Stream{
						PublicKey: `BCPcL\/paSqcvCgh2Uu6LbN7o67BFY0cnqfkNUiza62fMQzDFC\/zgB5eSA\/h6nXpuSpUIg7JSOBMoxkavrL5FOj4=`,
						Entitlements: []postgres.Entitlement{
							postgres.Entitlement{
								SensorID: 29,
								Action:   string(pipeline.Share),
							},
						},
					},
				},
			},
			encryptedPayloads: [][]byte{
				[]byte(`B5yRzPqlmh+4rmSnxNMJ1W8NBv8Om0TkvWraCx27u1H+yRL+P+DTZ6+YjoSKzvHeWEOStuJt7XnFY8H2QiTiLwjkmRoxBvzqCZ2ffbz7zAU7di8PzrRtRF/uRx0LgLrtzbfbJTgIiRvCr0D3x+nJIHIsI9TzEI0rAqT6uNcNnkNBmypcsFssRbwg1yRg4FIl53iIQ3A8c23B74NYy+24PRHidE+ncW7T2FxjhC8a4PB8UEaDcB0cmT4RJM+OX2hrzCBizv8mktAwbJKH5aHai92SybqV49sIFO48xJqnlINNHZh/pZZURFrS1lp4bKEaKOuh5G0mkxtvgADkhB0BVCKwnaFNpiy4NYddGqQZRcabYXoO+slBgMAOJZbU7tU6`),
			},
		},
		{
			label: "binning entitlement",
			device: &postgres.Device{
				UserUID:    "bob",
				PrivateKey: `Dmchv5dXlMgST8ZdXOaEHmT+HvSj44nvgd0PsiSL\/FE=`,
				Longitude:  2.5,
				Latitude:   2.5,
				Exposure:   "INDOOR",
				Streams: []*postgres.Stream{
					&postgres.Stream{
						PublicKey: `BCPcL\/paSqcvCgh2Uu6LbN7o67BFY0cnqfkNUiza62fMQzDFC\/zgB5eSA\/h6nXpuSpUIg7JSOBMoxkavrL5FOj4=`,
						Entitlements: []postgres.Entitlement{
							postgres.Entitlement{
								SensorID: 29,
								Action:   string(pipeline.Bin),
								Bins:     []float64{40, 80},
							},
						},
					},
				},
			},
			encryptedPayloads: [][]byte{
				[]byte(`B5yRzPqlmh+4rmSnxNMJ1W8NBv8Om0TkvWraCx27u1H+yRL+P+DTZ6+YjoSKzvHeWEOStuJt7XnFY8H2QiTiLwjkmRoxBvzqCZ2ffbz7zAU7di8PzrRtRF/uRx0LgLrtzbfbJTgIiRvCr0D3x+nJIHIsI9TzEI0rAqT6uNcNnkNBmypcsFssRbwg1yRg4FIl53iIQ3A8c23B74NYy+24PRHidE+ncW7T2FxjhC8a4PB8UEaDcB0cmT4RJM+OX2hrzCBizv8mktAwbJKH5aHai92SybqV49sIFO48xJqnlINNHZh/pZZURFrS1lp4bKEa74SwY/CQCkaKeRbl3k5lcK8dr5bT+MNeLhjyZjOhcOrvyq4ZpXXFALtzcRqdlgS79zMS6I9Wboh37Vy6Y5G9T2yOIaaDNUfFXBnhiiLwugk=`),
			},
		},
	}

	for _, tc := range testcases {
		t.Run(tc.label, func(t *testing.T) {
			ds := mocks.Datastore{}

			// set up mock responses
			for _, encryptedPayload := range tc.encryptedPayloads {
				ds.On(
					"WriteData",
					context.Background(),
					&datastore.WriteRequest{
						PublicKey: `BCPcL\/paSqcvCgh2Uu6LbN7o67BFY0cnqfkNUiza62fMQzDFC\/zgB5eSA\/h6nXpuSpUIg7JSOBMoxkavrL5FOj4=`,
						UserUid:   "bob",
						Data:      encryptedPayload,
					},
				).Return(
					&datastore.WriteResponse{},
					nil,
				)
			}

			// create new processor
			processor, err := pipeline.NewProcessor(&pipeline.Config{Verbose: true}, datastore.Datastore(&ds), logger)
			assert.Nil(t, err)

			// run the processor
			err = processor.Process(tc.device, payload)
			assert.Nil(t, err)

			// and assert on expectations
			ds.AssertExpectations(t)
		})
	}
}

func TestProcessWithError(t *testing.T) {
	t.Skip()
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

	processor, err := pipeline.NewProcessor(&pipeline.Config{Verbose: true}, datastore.Datastore(&ds), logger)
	assert.Nil(t, err)

	device := &postgres.Device{
		UserUID:    "bob",
		PrivateKey: `Dmchv5dXlMgST8ZdXOaEHmT+HvSj44nvgd0PsiSL\/FE=`,
		Streams: []*postgres.Stream{
			{
				PublicKey: `BCPcL\/paSqcvCgh2Uu6LbN7o67BFY0cnqfkNUiza62fMQzDFC\/zgB5eSA\/h6nXpuSpUIg7JSOBMoxkavrL5FOj4=`,
			},
		},
	}

	err = processor.Process(device, payload)
	assert.NotNil(t, err)
	assert.Equal(t, "error", err.Error())

	ds.AssertExpectations(t)
}

func TestBinnedValues(t *testing.T) {
	testcases := []struct {
		label    string
		value    float64
		bins     []float64
		expected []int
	}{
		{
			label:    "single bucket, first",
			value:    39.2,
			bins:     []float64{40},
			expected: []int{1, 0},
		},
		{
			label:    "single bucket, second",
			value:    67.12,
			bins:     []float64{40},
			expected: []int{0, 1},
		},
		{
			label:    "two buckets, first",
			value:    39.2,
			bins:     []float64{40, 60},
			expected: []int{1, 0, 0},
		},
		{
			label:    "two buckets, middle",
			value:    47.24,
			bins:     []float64{40, 60},
			expected: []int{0, 1, 0},
		},
		{
			label:    "two buckets, above",
			value:    67.24,
			bins:     []float64{40, 60},
			expected: []int{0, 0, 1},
		},
		{
			label:    "two buckets, on lower boundary",
			value:    40,
			bins:     []float64{40, 60},
			expected: []int{1, 0, 0},
		},
		{
			label:    "two buckets, on upper boundary",
			value:    60,
			bins:     []float64{40, 60},
			expected: []int{0, 1, 0},
		},
		{
			label:    "three buckets",
			value:    43.15,
			bins:     []float64{40, 60, 80},
			expected: []int{0, 1, 0, 0},
		},
	}

	for _, tc := range testcases {
		t.Run(tc.label, func(t *testing.T) {
			got := pipeline.BinnedValues(tc.value, tc.bins)
			assert.Equal(t, tc.expected, got)
		})
	}
}
