package pipeline_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/guregu/null"
	"github.com/stretchr/testify/assert"
	"github.com/thingful/iotencoder/pkg/pipeline"
)

func TestSensorMarshalling(t *testing.T) {
	testcases := []struct {
		label    string
		sensor   *pipeline.Sensor
		expected string
	}{
		{
			label: "simple share",
			sensor: &pipeline.Sensor{
				ID:          29,
				Name:        "MEMS Mic",
				Description: "MEMS microphone with envelope follower sound pressure sensor (noise)",
				Unit:        "dBC",
				Type:        pipeline.Share,
				Value:       null.FloatFrom(64.252),
			},
			expected: `{"id": 29,"name": "MEMS Mic","description":"MEMS microphone with envelope follower sound pressure sensor (noise)","unit": "dBC","type": "SHARE","value": 64.252,"interval":null}`,
		},
		{
			label: "binning",
			sensor: &pipeline.Sensor{
				ID:          12,
				Name:        "HPP828E031",
				Description: "Temperature",
				Unit:        "ºC",
				Type:        pipeline.Bin,
				Bins:        []float64{0, 15, 30},
				Values:      []int{0, 1, 0, 0},
			},
			expected: `{"id": 12,"name":"HPP828E031","description":"Temperature","unit":"ºC","type":"BIN","bins":[0,15,30],"values":[0,1,0,0],"value":null,"interval":null}`,
		},
		{
			label: "moving average",
			sensor: &pipeline.Sensor{
				ID:          12,
				Name:        "HPP828E031",
				Description: "Temperature",
				Unit:        "ºC",
				Type:        pipeline.MovingAvg,
				Interval:    null.IntFrom(900),
				Value:       null.FloatFrom(12.23),
			},
			expected: `{"id": 12,"name":"HPP828E031","description":"Temperature","unit":"ºC","type":"MOVING_AVG","value":12.23,"interval":900}`,
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.label, func(t *testing.T) {
			got, err := json.Marshal(testcase.sensor)
			assert.Nil(t, err)

			assert.JSONEq(t, testcase.expected, string(got))
		})
	}
}

func TestRawPayloadParsing(t *testing.T) {
	testcases := []struct {
		label    string
		input    string
		expected *pipeline.RawPayload
	}{
		{
			label: "valid",
			input: `{"data":[{"recorded_at": "2018-06-03T22:26:02Z","sensors":[{"id":10,"value":100},{"id":29,"value":67.02939},{"id":13,"value":74.65033},{"id":12,"value":22.41268},{"id":14,"value":5.29416}]}]}`,
			expected: &pipeline.RawPayload{
				Data: []pipeline.RawData{
					{
						RecordedAt: time.Date(2018, 6, 3, 22, 26, 2, 0, time.UTC),
						Sensors: []pipeline.RawSensor{
							{
								ID:    10,
								Value: 100,
							},
							{
								ID:    29,
								Value: 67.02939,
							},
							{
								ID:    13,
								Value: 74.65033,
							},
							{
								ID:    12,
								Value: 22.41268,
							},
							{
								ID:    14,
								Value: 5.29416,
							},
						},
					},
				},
			},
		},
	}

	for _, tc := range testcases {
		t.Run(tc.label, func(t *testing.T) {
			var payload pipeline.RawPayload
			err := json.Unmarshal([]byte(tc.input), &payload)
			assert.Nil(t, err)
		})
	}
}
