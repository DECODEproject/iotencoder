package smartcitizen_test

import (
	"encoding/json"
	"testing"
	"time"

	"gopkg.in/guregu/null.v3"

	"github.com/stretchr/testify/assert"

	"github.com/DECODEproject/iotencoder/pkg/postgres"
	"github.com/DECODEproject/iotencoder/pkg/smartcitizen"
)

func TestParseData(t *testing.T) {
	device := &postgres.Device{DeviceToken: "abc123", Longitude: 12, Latitude: 12, Exposure: "INDOOR"}

	payload := []byte(`{"data":[{"recorded_at":"2018-12-01T10:00:00Z","sensors":[{"id":12,"value":12.3},{"id":14,"value":23.2}]}]}`)

	s := smartcitizen.Smartcitizen{}

	unit1 := null.StringFrom("ºC")
	value1 := null.FloatFrom(12.3)

	unit2 := null.StringFrom("Lux")
	value2 := null.FloatFrom(23.2)

	expected := &smartcitizen.Device{
		Token:      "abc123",
		Longitude:  12,
		Latitude:   12,
		Exposure:   "INDOOR",
		RecordedAt: time.Date(2018, 12, 1, 10, 0, 0, 0, time.UTC),
		Sensors: []*smartcitizen.Sensor{
			&smartcitizen.Sensor{
				ID:          12,
				Name:        "HPP828E031",
				Description: "Temperature",
				Unit:        &unit1,
				Action:      postgres.Share,
				Value:       &value1,
			},
			&smartcitizen.Sensor{
				ID:          14,
				Name:        "BH1730FVC",
				Description: "Digital Ambient Light Sensor",
				Unit:        &unit2,
				Action:      postgres.Share,
				Value:       &value2,
			},
		},
	}

	got, err := s.ParseData(device, payload)
	assert.Nil(t, err)
	assert.Equal(t, expected, got)
}

func TestMarshalling(t *testing.T) {
	unit1 := null.StringFrom("ºC")
	value1 := null.FloatFrom(12.3)

	unit2 := null.StringFrom("Lux")
	interval2 := null.IntFrom(300)
	value2 := null.FloatFrom(23.2)

	unit3 := null.StringFrom("dBA")

	device := &smartcitizen.Device{
		Token:      "abc123",
		Longitude:  12,
		Latitude:   12,
		Exposure:   "INDOOR",
		RecordedAt: time.Date(2018, 12, 1, 10, 0, 0, 0, time.UTC),
		Sensors: []*smartcitizen.Sensor{
			&smartcitizen.Sensor{
				ID:          12,
				Name:        "HPP828E031",
				Description: "Temperature",
				Unit:        &unit1,
				Action:      postgres.Share,
				Value:       &value1,
			},
			&smartcitizen.Sensor{
				ID:          14,
				Name:        "BH1730FVC",
				Description: "Digital Ambient Light Sensor",
				Unit:        &unit2,
				Action:      postgres.MovingAverage,
				Interval:    &interval2,
				Value:       &value2,
			},
			&smartcitizen.Sensor{
				ID:          53,
				Name:        "dBA1",
				Description: "Digital Sound Sensor",
				Unit:        &unit3,
				Action:      postgres.Bin,
				Bins:        []float64{30, 60, 90},
				Values:      []int{0, 1, 0, 0},
			},
		},
	}

	b, err := json.Marshal(device)
	assert.Nil(t, err)

	expected := `{"token":"abc123","longitude":12,"latitude":12,"exposure":"INDOOR","recordedAt":"2018-12-01T10:00:00Z","sensors":[{"id":12,"name":"HPP828E031","description":"Temperature","unit":"ºC","type":"SHARE","value":12.3},{"id":14,"name":"BH1730FVC","description":"Digital Ambient Light Sensor","unit":"Lux","type":"MOVING_AVG","interval":300,"value":23.2},{"id":53,"name":"dBA1","description":"Digital Sound Sensor","unit":"dBA","type":"BIN","bins":[30,60,90],"values":[0,1,0,0]}]}`

	assert.Equal(t, expected, string(b))
}
