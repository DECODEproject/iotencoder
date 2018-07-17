package pipeline

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	kitlog "github.com/go-kit/kit/log"
	"github.com/go-redis/redis"
	"github.com/guregu/null"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	datastore "github.com/thingful/twirp-datastore-go"
	"github.com/thingful/zenroom-go"

	"github.com/thingful/iotencoder/pkg/lua"
	"github.com/thingful/iotencoder/pkg/postgres"
)

var (
	// pipelineErrorCounter is a counter vec used to count any errors that happen
	// while processing incoming payloads.
	pipelineErrorCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "decode_pipeline_errors",
			Help: "Count of errors while processing the pipeline",
		},
		[]string{"cause"},
	)

	// datastoreWriteHistogram is a prometheus histogram recording successful
	// writes to the datastore. We use the default bucket distributions.
	datastoreWriteHistogram = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name: "decode_datastore_writes",
			Help: "Datastore writes duration distribution",
		},
	)

	// zenroomHistogram is a prometheus histogram recording execution times of
	// calls to zenroom to exec some script.
	zenroomHistogram = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name: "decode_zenroom_exec",
			Help: "Execution time of zenroom scripts",
		},
	)
)

func init() {
	prometheus.MustRegister(pipelineErrorCounter)
	prometheus.MustRegister(datastoreWriteHistogram)
	prometheus.MustRegister(zenroomHistogram)
}

type Config struct {
	RedisAddr     string
	RedisPassword string
	RedisDB       int
	Verbose       bool
}

// Processor is an interface we define to handle processing all the streams for
// a device, where processing means reading all streams for the device, applying
// whatever operations that stream specifies in terms of filtering / aggregation
// / bucketing, encrypting the result and then writing the encrypted body to the
// datastore.
type Processor interface {
	// Process takes an input a device which will have one or more attached
	// streams, as well as the received payload from the device. Internally it is
	// responsible for processing the data for each stream and then writing the
	// encrypted result to the remote datastore.
	Process(device *postgres.Device, payload []byte) error
}

// processor is our internal type that implements the above interface
type processor struct {
	datastore datastore.Datastore
	logger    kitlog.Logger
	verbose   bool
	sensors   map[int]SensorInfo
	redis     *redis.Client
}

// NewProcessor is a constructor function that takes as input an instantiated
// datastore client, and a logger. It returns the instantiated processor which
// is ready for use. Note we pass in the datastore instance so that we can
// supply a mock for testing.
func NewProcessor(config *Config, ds datastore.Datastore, logger kitlog.Logger) (Processor, error) {
	logger = kitlog.With(logger, "module", "pipeline")

	client := redis.NewClient(&redis.Options{
		Addr:     config.RedisAddr,
		Password: config.RedisPassword,
		DB:       config.RedisDB,
	})

	// load sensor data from json asset
	sensorData, err := Asset("sensors.json")
	if err != nil {
		return nil, errors.Wrap(err, "failed to load sensors.json")
	}

	// unmarshal the json
	var sensorInfo []SensorInfo
	err = json.Unmarshal(sensorData, &sensorInfo)
	if err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal sensors.json")
	}

	// and create map keyed by sensor ID
	sensors := make(map[int]SensorInfo)
	for _, sensor := range sensorInfo {
		sensors[sensor.ID] = sensor
	}

	return &processor{
		datastore: ds,
		logger:    logger,
		verbose:   config.Verbose,
		sensors:   sensors,
		redis:     client,
	}, nil
}

// Process is the function that actually does the work of dispatching the
// received data to all destination streams after applying whatever processing
// the stream specifies. Currently we do the simplest thing of just writing the
// data directly to the datastore.
func (p *processor) Process(device *postgres.Device, data []byte) error {

	// check payload
	if data == nil || len(data) == 0 {
		pipelineErrorCounter.With(prometheus.Labels{"cause": "emptyBody"}).Inc()
		return errors.New("empty payload received")
	}

	var rawPayload RawPayload
	err := json.Unmarshal(data, &rawPayload)
	if err != nil {
		pipelineErrorCounter.With(prometheus.Labels{"cause": "unmarshalPayload"}).Inc()
		return errors.Wrap(err, "failed to unmarshal incoming json")
	}

	if len(rawPayload.Data) != 1 {
		return fmt.Errorf("Unexpected data length, expected: 1, got: %v\n", len(rawPayload.Data))
	}

	// read script from go-bindata asset
	script, err := lua.Asset("encrypt.lua")
	if err != nil {
		pipelineErrorCounter.With(prometheus.Labels{"cause": "zenroomScript"}).Inc()
		return errors.Wrap(err, "failed to read zenroom script")
	}

	for _, stream := range device.Streams {
		start := time.Now()

		payload, err := Process(&rawPayload, device, stream, p.sensors)
		if err != nil {
			pipelineErrorCounter.With(prometheus.Labels{"cause": "processing"}).Inc()
			p.logger.Log("err", err, "msg", "failed to process payload")
			continue
		}

		// note we have to base64 encode our payload as it may have non-ASCII
		// characters after metadata processing.
		encodedPayload, err := zenroom.Exec(
			script,
			[]byte(fmt.Sprintf(`{"public":"%s","private":"%s"}`, stream.PublicKey, device.PrivateKey)),
			[]byte(base64.StdEncoding.EncodeToString(payload)),
		)

		duration := time.Since(start)

		if err != nil {
			pipelineErrorCounter.With(prometheus.Labels{"cause": "zenroom"}).Inc()
			p.logger.Log("err", err, "msg", "failed to encrypt payload")
			continue
		}

		zenroomHistogram.Observe(duration.Seconds())

		if p.verbose {
			p.logger.Log(
				"public_key", stream.PublicKey,
				"user_uid", device.UserUID,
				"processedPayload", string(payload),
				"encodedPayload", string(encodedPayload),
				"msg", "writing data",
			)
		}

		start = time.Now()

		_, err = p.datastore.WriteData(context.Background(), &datastore.WriteRequest{
			PublicKey: stream.PublicKey,
			UserUid:   device.UserUID,
			Data:      encodedPayload,
		})

		duration = time.Since(start)

		if err != nil {
			pipelineErrorCounter.With(prometheus.Labels{"cause": "datastore"}).Inc()
			p.logger.Log("err", err, "msg", "failed to write to datstore")
			continue
		}

		datastoreWriteHistogram.Observe(duration.Seconds())
	}

	return nil
}

// Process takes as input a RawPayload instance, as well as the current Device
// and Stream. The passed in Device contains the device metadata, and the Stream
// defines a list of operations to apply to the received data
func Process(rawPayload *RawPayload, device *postgres.Device, stream *postgres.Stream, sensors map[int]SensorInfo) ([]byte, error) {
	data := rawPayload.Data[0]

	payload := Payload{
		Location: Location{
			Longitude: device.Longitude,
			Latitude:  device.Latitude,
			Exposure:  device.Exposure,
		},
		RecordedAt: data.RecordedAt,
		Sensors:    []Sensor{},
	}

	if len(stream.Entitlements) == 0 {
		// add all channels to the output
		for _, rawSensor := range data.Sensors {
			if sensorInfo, ok := sensors[rawSensor.ID]; ok {
				sensor := Sensor{
					ID:          rawSensor.ID,
					Name:        sensorInfo.Name,
					Description: sensorInfo.Description,
					Unit:        sensorInfo.Unit,
					Type:        Share,
					Value:       null.FloatFrom(rawSensor.Value),
				}

				payload.Sensors = append(payload.Sensors, sensor)
			}
		}
	} else {
		// iterate over entitlements
		for _, entitlement := range stream.Entitlements {
			if sensorInfo, ok := sensors[entitlement.SensorID]; ok {
				rawSensor, err := findSensor(entitlement.SensorID, data.Sensors)
				if err != nil {
					return nil, err
				}

				sensor, err := makeProcessedSensor(&entitlement, &sensorInfo, rawSensor)
				if err != nil {
					return nil, errors.Wrap(err, "failed to generate processed sensor")
				}

				payload.Sensors = append(payload.Sensors, *sensor)
			}
		}
	}

	return json.Marshal(payload)
}

func makeProcessedSensor(entitlement *postgres.Entitlement, sensorInfo *SensorInfo, rawSensor *RawSensor) (*Sensor, error) {
	sensor := Sensor{
		ID:          entitlement.SensorID,
		Name:        sensorInfo.Name,
		Description: sensorInfo.Description,
		Unit:        sensorInfo.Unit,
	}

	switch entitlement.Action {
	case string(Share):
		sensor.Type = Share
		sensor.Value = null.FloatFrom(rawSensor.Value)
	case string(Bin):
		sensor.Type = Bin
		sensor.Bins = entitlement.Bins
		sensor.Values = BinnedValues(rawSensor.Value, entitlement.Bins)
	case string(MovingAvg):
		sensor.Type = MovingAvg
		sensor.Interval = entitlement.Interval
	default:
		return nil, fmt.Errorf("Unknown entitlement action: %s", entitlement.Action)
	}

	return &sensor, nil
}

func findSensor(sensorID int, rawSensors []RawSensor) (*RawSensor, error) {
	for _, rawSensor := range rawSensors {
		if sensorID == rawSensor.ID {
			return &rawSensor, nil
		}
	}

	return nil, fmt.Errorf("Unable to find sensor with id: %v", sensorID)
}

// BinnedValues returns a slice containing the binned output for the given
// parameters. We pass in the received value as well as a slice which defines
// the bin boundaries. We define the boundaries slice where each value
// represents the upper inclusive boundary of a bin. There is an implicit +Inf
// bound after the last value.
func BinnedValues(value float64, bins []float64) []int {
	// example
	// value: 67.212
	// bins: [40,80]
	// expected ouput: [0,1,0]

	binnedVals := make([]int, len(bins)+1)

	for i, _ := range binnedVals {
		switch {
		case i == 0:
			if value <= bins[i] {
				binnedVals[i] = 1
				continue
			}
			binnedVals[i] = 0
		case i == len(binnedVals)-1:
			if value > bins[i-1] {
				binnedVals[i] = 1
				continue
			}
			binnedVals[i] = 0
		default:
			if value > bins[i-1] && value <= bins[i] {
				binnedVals[i] = 1
				continue
			}
			binnedVals[i] = 0
		}
	}

	return binnedVals
}
