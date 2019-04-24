package mocks

import (
	"github.com/stretchr/testify/mock"
)

type MovingAverager struct {
	mock.Mock
}

func (m *MovingAverager) MovingAverage(value float64, deviceToken string, sensorID int, interval uint32) (float64, error) {
	args := m.Called(value, deviceToken, sensorID, interval)
	return args.Get(0).(float64), args.Error(1)
}
