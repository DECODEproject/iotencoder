package pipeline

import (
	"context"

	kitlog "github.com/go-kit/kit/log"
	"github.com/thingful/iotencoder/pkg/postgres"

	datastore "github.com/thingful/twirp-datastore-go"
)

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
}

// NewProcessor is a constructor function that takes as input an instantiated
// datastore client, and a logger. It returns the instantiated processor which
// is ready for use. Note we pass in the datastore instance so that we can
// supply a mock for testing.
func NewProcessor(ds datastore.Datastore, logger kitlog.Logger) Processor {
	logger = kitlog.With(logger, "module", "pipeline")

	logger.Log("msg", "creating processor")

	return &processor{
		datastore: ds,
		logger:    logger,
	}
}

// Process is the function that actually does the work of dispatching the
// received data to all destination streams after applying whatever processing
// the stream specifies. Currently we do the simplest thing of just writing the
// data directly to the datastore.
func (p *processor) Process(device *postgres.Device, payload []byte) error {
	for _, stream := range device.Streams {
		_, err := p.datastore.WriteData(context.Background(), &datastore.WriteRequest{
			PublicKey: stream.PublicKey,
			UserUid:   device.UserUID,
			Data:      payload,
		})

		if err != nil {
			return err
		}
	}

	return nil
}
