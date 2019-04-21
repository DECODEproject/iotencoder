package clock_test

import (
	"testing"
	"time"

	"github.com/DECODEproject/iotencoder/pkg/clock"
	"github.com/stretchr/testify/assert"
)

func TestMockClock(t *testing.T) {
	now := time.Now()

	c := clock.NewMock(now)
	assert.Equal(t, now, c.Now())

	after := time.Now()
	c.Set(after)

	assert.NotEqual(t, now, c.Now())
	assert.Equal(t, after, c.Now())

	c.Add(1 * time.Hour)
	assert.NotEqual(t, after, c.Now())
}
