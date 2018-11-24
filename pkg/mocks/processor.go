package mocks

import (
	"github.com/DECODEproject/iotencoder/pkg/postgres"
)

type Processor struct{}

func NewProcessor() *Processor {
	return &Processor{}
}

func (p *Processor) Process(device *postgres.Device, payload []byte) error {
	return nil
}
