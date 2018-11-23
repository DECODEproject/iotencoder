package smartcitizen

import "time"

// RawSensor is a type used when parsing the actual payload published by
// Smartcitizen
type RawSensor struct {
	ID    int     `json:"id"`
	Value float64 `json:"value"`
}

// SensorData is a type used when parsing the actual payload published by
// Smartcitizen
type SensorData struct {
	RecordedAt time.Time   `json:"recorded_at"`
	Sensors    []RawSensor `json:"sensors"`
}

// Payload is a type used when parsing the actual payload published by
// Smartcitizen
type Payload struct {
	Data []SensorData `json:"data"`
}
