package smartcitizen_test

import (
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
				Unit:        null.StringFrom("ºC"),
				Action:      postgres.Share,
				Value:       null.FloatFrom(12.3),
			},
			&smartcitizen.Sensor{
				ID:          14,
				Name:        "BH1730FVC",
				Description: "Digital Ambient Light Sensor",
				Unit:        null.StringFrom("Lux"),
				Action:      postgres.Share,
				Value:       null.FloatFrom(23.2),
			},
		},
	}

	got, err := s.ParseData(device, payload)
	assert.Nil(t, err)
	assert.Equal(t, expected, got)
}
