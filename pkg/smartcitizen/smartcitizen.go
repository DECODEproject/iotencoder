package smartcitizen

import (
	"encoding/json"
	"time"

	"github.com/pkg/errors"
	encoder "github.com/thingful/twirp-encoder-go"
	"gopkg.in/guregu/null.v3"

	"github.com/DECODEproject/iotencoder/pkg/postgres"
)

// Sensor is a type used when we marshal the enriched data to write to the
// datastore
type Sensor struct {
	ID          int             `json:"id"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Unit        *null.String    `json:"unit,omitempty"`
	Action      postgres.Action `json:"type"`
	Interval    *null.Int       `json:"interval,omitempty"`
	Value       *null.Float     `json:"value,omitempty"`
	Bins        []float64       `json:"bins,omitempty"`
	Values      []int           `json:"values,omitempty"`
}

// Device is a type used when we marshal the enriched data to write to the
// datastore.
type Device struct {
	Token      string    `json:"token"`
	Label      string    `json:"label"`
	Longitude  float64   `json:"longitude"`
	Latitude   float64   `json:"latitude"`
	Exposure   string    `json:"exposure"`
	RecordedAt time.Time `json:"recordedAt"`
	Sensors    []*Sensor `json:"sensors"`
}

// FindSensor is a helper function that either returns a sensor pointer from our
// slice, or returns nil if the sensor identified by the given id is not found.
func (d *Device) FindSensor(id int) *Sensor {
	for _, sensor := range d.Sensors {
		if sensor.ID == id {
			return sensor
		}
	}
	return nil
}

// Smartcitizen is our type that holds the map of sensor metadata, and is able
// to use this state to enrich an incoming payload.
type Smartcitizen struct {
	sensorMetadata map[int]SensorMetadata
}

// ParseData is our main public function, that takes in the device
// representation from our database and the bytes of the payload. It then parses
// this payload into an internal representation, which we then enrich using the
// metadata, before returning an object containing the additional richer data.
func (s *Smartcitizen) ParseData(device *postgres.Device, payload []byte) (*Device, error) {
	if s.sensorMetadata == nil {
		sensorMetadata, err := ReadMetadata()
		if err != nil {
			return nil, errors.Wrap(err, "failed to read sensor metadata")
		}
		s.sensorMetadata = sensorMetadata
	}

	var p Payload
	err := json.Unmarshal(payload, &p)
	if err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal raw payload")
	}

	if len(p.Data) == 0 {
		return nil, errors.New("missing data from payload")
	}

	data := p.Data[0]

	d := &Device{
		Token:      device.DeviceToken,
		Label:      device.Label,
		Longitude:  device.Longitude,
		Latitude:   device.Latitude,
		Exposure:   device.Exposure,
		RecordedAt: data.RecordedAt,
		Sensors:    []*Sensor{},
	}

	for _, rawSensor := range data.Sensors {
		metadata, ok := s.sensorMetadata[rawSensor.ID]
		if !ok {
			continue
		}

		value := null.FloatFrom(rawSensor.Value)

		sensor := &Sensor{
			ID:          rawSensor.ID,
			Name:        metadata.Name,
			Description: metadata.Description,
			Value:       &value,
			Action:      postgres.Action(encoder.CreateStreamRequest_Operation_SHARE.String()),
			Unit:        &metadata.Unit,
		}

		d.Sensors = append(d.Sensors, sensor)
	}

	return d, nil
}
