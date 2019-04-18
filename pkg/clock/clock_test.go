package clock_test

import (
	"testing"

	"github.com/DECODEproject/iotencoder/pkg/clock"
	"github.com/stretchr/testify/assert"
)

func TestRealClock(t *testing.T) {
	c := clock.New()
	assert.NotNil(t, c)

	now := c.Now()
	assert.False(t, now.IsZero())
}
