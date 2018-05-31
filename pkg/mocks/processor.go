package mocks

import (
	"github.com/thingful/iotencoder/pkg/pipeline"
	"github.com/thingful/iotencoder/pkg/postgres"
)

type processor struct{}

func NewProcessor() pipeline.Processor {
	return &processor{}
}

func (p *processor) Process(device *postgres.Device, payload []byte) error {
	return nil
}
