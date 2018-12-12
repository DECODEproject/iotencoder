package mocks

import (
	"github.com/stretchr/testify/mock"
)

// Redis is our mock redis client
type Redis struct {
	mock.Mock
}

func (r *Redis) MovingAverage(value float64, deviceToken string, sensorID int, interval uint32) (float64, error) {
	args := r.Called(value, deviceToken, sensorID, interval)
	return args.Get(0).(float64), args.Error(1)
}
