package mocks

import (
	"time"

	"github.com/stretchr/testify/mock"
)

type Clock struct {
	mock.Mock
}

func (c *Clock) Now() time.Time {
	args := c.Called()
	return args.Get(0).(time.Time)
}
