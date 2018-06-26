package pipeline

import (
	"time"

	"github.com/guregu/null"
)

// SensorType is a type alias used to ensure we use the correct value when
// outputting sensor type
type SensorType string

const (
	// Share means share a sensor at full resolution
	Share SensorType = "SHARE"

	// Bin means share a sensor after applying a binning filter
	Bin SensorType = "BIN"

	// MovingAvg means share a sensor after applying a moving average
	MovingAvg SensorType = "MOVING_AVG"
)

// Payload is our primary outgoing data type. It contains fields for the owner,
// the location of the device, the timestamp at which the event was recorded,
// and an array of sensor instances containing the actual data.
type Payload struct {
	Location   Location  `json:"location"`
	RecordedAt time.Time `json:"recordedAt"`
	Sensors    []Sensor  `json:"sensors"`
}

// Location is a struct used to hold the location of the device.
type Location struct {
	Longitude float64 `json:"longitude"`
	Latitude  float64 `json:"latitude"`
	Exposure  string  `json:"exposure"`
}

// Sensor is the struct that holds the actual data values for each sensor. It is
// designed to be able to output JSON for either SHARE, BIN, or MOVING_AVG
// depending on how the stream is configured.
type Sensor struct {
	ID          int        `json:"id"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Unit        string     `json:"unit"`
	Type        SensorType `json:"type"`
	Value       null.Float `json:"value,omitempty"`
	Interval    null.Int   `json:"interval,omitempty"`
	Bins        []float64  `json:"bins,omitempty"`
	Values      []int      `json:"values,omitempty"`
}

// RawSensor is our type used when parsing data received from the SmartCitizen
// MQTT broker. Contains no metadata, just the sensor id and value.
type RawSensor struct {
	ID    int     `json:"id"`
	Value float64 `json:"value"`
}

// RawData is part of the payload we receive from SmartCitizen's MQTT broker.
type RawData struct {
	RecordedAt time.Time   `json:"recorded_at"`
	Sensors    []RawSensor `json:"sensors"`
}

// RawPayload is a struct used to decode the raw payload received from
// SmartCitizen's MQTT broker. It contains a single attribute data, which
// contains an array containing a single RawData instance.
type RawPayload struct {
	Data []RawData `json:"data"`
}
