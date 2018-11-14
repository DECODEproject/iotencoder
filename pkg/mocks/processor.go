package mocks

import (
	"github.com/DECODEproject/iotencoder/pkg/pipeline"
	"github.com/DECODEproject/iotencoder/pkg/postgres"
)

type processor struct{}

func NewProcessor() pipeline.Processor {
	return &processor{}
}

func (p *processor) Process(device *postgres.Device, payload []byte) error {
	return nil
}
