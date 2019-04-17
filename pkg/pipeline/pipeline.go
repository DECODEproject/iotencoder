package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	zenroom "github.com/DECODEproject/zenroom-go"
	kitlog "github.com/go-kit/kit/log"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	datastore "github.com/thingful/twirp-datastore-go"
	"gopkg.in/guregu/null.v3"

	"github.com/DECODEproject/iotencoder/pkg/lua"
	"github.com/DECODEproject/iotencoder/pkg/postgres"
	"github.com/DECODEproject/iotencoder/pkg/smartcitizen"
)

var (
	// DatastoreErrorCounter is a prometheus counter recording a count of any
	// errors that occur when writing to the datastore
	DatastoreErrorCounter = prometheus.NewCounter(
		prometheus.CounterOpts{
			Namespace: "decode",
			Subsystem: "encoder",
			Name:      "datastore_errors",
			Help:      "Count of errors writing to datastore",
		},
	)

	// ZenroomErrorCounter is a prometheus counter recording a count of any errors
	// that occur when invoking zenroom.
	ZenroomErrorCounter = prometheus.NewCounter(
		prometheus.CounterOpts{
			Namespace: "decode",
			Subsystem: "encoder",
			Name:      "zenroom_errors",
			Help:      "Count of errors invoking zenroom",
		},
	)

	// DatastoreWriteHistogram is a prometheus histogram recording successful
	// writes to the datastore. We use the default bucket distributions.
	DatastoreWriteHistogram = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Namespace: "decode",
			Subsystem: "encoder",
			Name:      "datastore_writes",
			Help:      "Datastore writes duration distribution",
		},
	)

	// ProcessHistogram is a prometheus histogram recording duration of processing
	// a device for a stream.
	ProcessHistogram = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "decode",
			Subsystem: "encoder",
			Name:      "pipeline_process",
			Help:      "Execution time of pipeline process",
		},
		[]string{"operation"},
	)

	// ZenroomHistogram is a prometheus histogram recording execution times of
	// calls to zenroom to exec some script.
	ZenroomHistogram = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Namespace: "decode",
			Subsystem: "encoder",
			Name:      "zenroom_exec",
			Help:      "Execution time of zenroom scripts",
		},
	)
)

// MovingAverager is an interface for a type that can return a moving average
// for the given device/sensor/interval
type MovingAverager interface {
	MovingAverage(value float64, deviceToken string, sensorId int, interval uint32) (float64, error)
}

// Processor is a type that encapsulates processing incoming events received
// from smartcitizen, and is responsible for enriching the data, applying any
// transformations to the data and then encrypting it using zenroom before
// writing it to the datastore.
type Processor struct {
	datastore datastore.Datastore
	logger    kitlog.Logger
	verbose   bool
	sensors   *smartcitizen.Smartcitizen
	movingAvg MovingAverager
}

// NewProcessor is a constructor function that takes as input an instantiated
// datastore client, and a logger. It returns the instantiated processor which
// is ready for use. Note we pass in the datastore instance so that we can
// supply a mock for testing.
func NewProcessor(ds datastore.Datastore, movingAvg MovingAverager, verbose bool, logger kitlog.Logger) *Processor {
	logger = kitlog.With(logger, "module", "pipeline")

	return &Processor{
		datastore: ds,
		logger:    logger,
		verbose:   verbose,
		sensors:   &smartcitizen.Smartcitizen{},
		movingAvg: movingAvg,
	}
}

// Process is the function that actually does the work of dispatching the
// received data to all destination streams after applying whatever processing
// the stream specifies. Currently we do the simplest thing of just writing the
// data directly to the datastore.
func (p *Processor) Process(device *postgres.Device, payload []byte) error {
	// check payload
	if payload == nil {
		return errors.New("empty payload received")
	}

	parsedDevice, err := p.sensors.ParseData(device, payload)
	if err != nil {
		return errors.Wrap(err, "failed to parse SmartCitizen data")
	}

	// pull encryption script from go-bindata asset
	script, err := lua.Asset("encrypt.lua")
	if err != nil {
		return errors.Wrap(err, "failed to read zenroom script")
	}

	// iterate over the configured streams for the device
	for _, stream := range device.Streams {
		if p.verbose {
			p.logger.Log("public_key", stream.PublicKey, "device_token", device.DeviceToken, "msg", "writing data")
		}

		keyString := fmt.Sprintf(
			`{"device_token":"%s","community_id":"%s","community_pubkey":"%s"}`,
			device.DeviceToken,
			stream.CommunityID,
			stream.PublicKey,
		)

		payloadBytes, err := p.processDevice(parsedDevice, stream)
		if err != nil {
			return err
		}

		if p.verbose {
			p.logger.Log("full_payload", string(payloadBytes))
		}

		start := time.Now()

		encodedPayload, err := zenroom.Exec(
			script,
			zenroom.WithKeys([]byte(keyString)),
			zenroom.WithData(payloadBytes),
			zenroom.WithVerbosity(1),
		)

		duration := time.Since(start)

		if err != nil {
			ZenroomErrorCounter.Inc()
			return err
		}

		ZenroomHistogram.Observe(duration.Seconds())

		start = time.Now()

		_, err = p.datastore.WriteData(context.Background(), &datastore.WriteRequest{
			CommunityId: stream.CommunityID,
			DeviceToken: device.DeviceToken,
			Data:        []byte(encodedPayload),
		})

		duration = time.Since(start)

		if err != nil {
			DatastoreErrorCounter.Inc()
			return err
		}

		DatastoreWriteHistogram.Observe(duration.Seconds())
	}

	return nil
}

func (p *Processor) processDevice(device *smartcitizen.Device, stream *postgres.Stream) ([]byte, error) {
	// if no operations just return the whole object
	if len(stream.Operations) == 0 {
		b, err := json.Marshal(device)
		if err != nil {
			return nil, errors.Wrap(err, "failed to marshal complete device")
		}
		return b, nil
	}

	// create empty slice for processed sensors
	processedSensors := []*smartcitizen.Sensor{}

	for _, operation := range stream.Operations {
		// get the sensor from the parsed slice
		sensor := device.FindSensor(int(operation.SensorID))

		if sensor != nil {
			switch operation.Action {
			case postgres.Share:
				start := time.Now()

				processedSensor := &smartcitizen.Sensor{
					ID:          sensor.ID,
					Name:        sensor.Name,
					Description: sensor.Description,
					Unit:        sensor.Unit,
					Action:      operation.Action,
					Value:       sensor.Value,
				}

				duration := time.Since(start)

				ProcessHistogram.WithLabelValues(string(postgres.Share)).Observe(duration.Seconds() * 1e3)

				processedSensors = append(processedSensors, processedSensor)
			case postgres.Bin:
				start := time.Now()

				processedSensor := &smartcitizen.Sensor{
					ID:          sensor.ID,
					Name:        sensor.Name,
					Description: sensor.Description,
					Unit:        sensor.Unit,
					Action:      operation.Action,
					Bins:        operation.Bins,
					Values:      BinValue(sensor.Value.Float64, operation.Bins),
				}

				duration := time.Since(start)

				ProcessHistogram.WithLabelValues(string(postgres.Bin)).Observe(duration.Seconds() * 1e3)

				processedSensors = append(processedSensors, processedSensor)
			case postgres.MovingAverage:
				start := time.Now()

				avgVal, err := p.movingAvg.MovingAverage(
					sensor.Value.Float64,
					device.Token,
					sensor.ID,
					operation.Interval,
				)
				if err != nil {
					return nil, errors.Wrap(err, "failed to calculate moving average")
				}

				interval := null.IntFrom(int64(operation.Interval))
				value := null.FloatFrom(avgVal)

				processedSensor := &smartcitizen.Sensor{
					ID:          sensor.ID,
					Name:        sensor.Name,
					Description: sensor.Description,
					Unit:        sensor.Unit,
					Action:      operation.Action,
					Interval:    &interval,
					Value:       &value,
				}

				duration := time.Since(start)

				ProcessHistogram.WithLabelValues(string(postgres.MovingAverage)).Observe(duration.Seconds() * 1e3)

				processedSensors = append(processedSensors, processedSensor)
			default:
				continue
			}
		}
	}

	device.Sensors = processedSensors

	b, err := json.Marshal(device)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal processed device")
	}

	return b, nil
}

// BinValue is a function that tuns a value and a slice containing bin
// boundaries into a slice containing the binned value.
func BinValue(value float64, bins []float64) []int {
	binnedValues := make([]int, len(bins)+1)

	classified := false

	for i := range bins {
		if i == 0 {
			if value < bins[i] {
				binnedValues[i] = 1
				classified = true
				break
			}
		} else {
			if value < bins[i] && value >= bins[i-1] {
				binnedValues[i] = 1
				classified = true
				break
			}
		}
	}

	if !classified {
		binnedValues[len(bins)] = 1
	}

	return binnedValues
}
