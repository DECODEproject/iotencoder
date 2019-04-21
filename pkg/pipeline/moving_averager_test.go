package pipeline_test

import (
	"testing"
	"time"

	kitlog "github.com/go-kit/kit/log"
	"github.com/stretchr/testify/assert"

	"github.com/DECODEproject/iotencoder/pkg/clock"
	"github.com/DECODEproject/iotencoder/pkg/pipeline"
)

func TestMovingAverager(t *testing.T) {
	logger := kitlog.NewNopLogger()

	cl := clock.NewMock(time.Now())
	mv := pipeline.NewMovingAverager(false, cl, logger)
	assert.NotNil(t, mv)

	avg, err := mv.MovingAverage(4.5, "abc123", 55, uint32(900))
	assert.Nil(t, err)
	assert.Equal(t, 4.5, avg)

	cl.Add(5 * time.Minute)
	avg, err = mv.MovingAverage(5.5, "abc123", 55, uint32(900))
	assert.Nil(t, err)
	assert.Equal(t, 5.0, avg)

	// spam another series so we can test it doesn't affect
	avg, err = mv.MovingAverage(2.2, "abc123", 12, uint32(900))
	assert.Nil(t, err)
	assert.Equal(t, 2.2, avg)

	cl.Add(5 * time.Minute)
	avg, err = mv.MovingAverage(6.5, "abc123", 55, uint32(900))
	assert.Nil(t, err)
	assert.Equal(t, 5.5, avg)

	cl.Add(5 * time.Minute)
	avg, err = mv.MovingAverage(5.5, "abc123", 55, uint32(900))
	assert.Nil(t, err)
	assert.Equal(t, 5.5, avg)

	cl.Add(5 * time.Minute)
	avg, err = mv.MovingAverage(1.2, "abc123", 55, uint32(900))
	assert.Nil(t, err)
	assert.Equal(t, 4.675, avg)
}
