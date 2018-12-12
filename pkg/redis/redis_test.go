package redis_test

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	kitlog "github.com/go-kit/kit/log"
	rd "github.com/go-redis/redis"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"

	"github.com/DECODEproject/iotencoder/pkg/mocks"
	"github.com/DECODEproject/iotencoder/pkg/redis"
)

func TestBuildKey(t *testing.T) {
	key := redis.BuildKey("abc123", 12, uint32(300))
	assert.Equal(t, "abc123:12:300", key)
}

func TestCalculateAverage(t *testing.T) {
	testcases := []struct {
		label    string
		input    []string
		expected float64
		err      error
	}{
		{
			label:    "empty list",
			input:    []string{},
			expected: 0.0,
			err:      nil,
		},
		{
			label:    "single value list",
			input:    []string{"12.2:1234567"},
			expected: 12.2,
			err:      nil,
		},
		{
			label:    "list of values",
			input:    []string{"12.2:1234567", "15.3:1234568", "8.8:12345669"},
			expected: 12.1,
			err:      nil,
		},
		{
			label:    "invalid element",
			input:    []string{"12.2:1234567", "13.9"},
			expected: 0.0,
			err:      errors.New("invalid value pulled from sorted set"),
		},
		{
			label:    "invalid number",
			input:    []string{"foo:1234567"},
			expected: 0.0,
			err:      errors.New("failed to parse float value read from sorted set: strconv.ParseFloat: parsing \"foo\": invalid syntax"),
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.label, func(t *testing.T) {
			got, err := redis.CalculateAverage(testcase.input)
			if testcase.err != nil {
				assert.NotNil(t, err)
				assert.Equal(t, testcase.err.Error(), err.Error())
			} else {
				assert.Nil(t, err)
				assert.Equal(t, testcase.expected, got)
			}
		})
	}
}

type RedisSuite struct {
	suite.Suite
	rd     *redis.Redis
	client *rd.Client
}

func (s *RedisSuite) SetupTest() {
	logger := kitlog.NewNopLogger()
	connStr := os.Getenv("IOTENCODER_REDIS_URL")

	now := time.Date(2018, 12, 12, 12, 0, 0, 0, time.UTC)

	clock := &mocks.Clock{}
	clock.On(
		"Now",
	).Return(
		now,
	)

	opt, err := rd.ParseURL(connStr)
	if err != nil {
		s.T().Fatalf("Failed to parse redis url: %v", err)
	}

	client := rd.NewClient(opt)
	_, err = client.FlushDb().Result()
	if err != nil {
		s.T().Fatalf("Failed to flush db: %v", err)
	}

	// populate sorted set with some values
	firstTime := now.Add(time.Minute * time.Duration(-20))
	secondTime := now.Add(time.Minute * time.Duration(-10))
	thirdTime := now.Add(time.Minute * time.Duration(-5))
	fourthTime := now.Add(time.Minute * time.Duration(-1))

	_, err = client.ZAdd(
		"abc123:12:900",
		rd.Z{
			Score:  float64(firstTime.Unix()),
			Member: fmt.Sprintf("%v:%v", 4.5, firstTime.Unix()),
		},
		rd.Z{
			Score:  float64(secondTime.Unix()),
			Member: fmt.Sprintf("%v:%v", 5.5, secondTime.Unix()),
		},
		rd.Z{
			Score:  float64(thirdTime.Unix()),
			Member: fmt.Sprintf("%v:%v", 6.5, thirdTime.Unix()),
		},
		rd.Z{
			Score:  float64(fourthTime.Unix()),
			Member: fmt.Sprintf("%v:%v", 3.5, fourthTime.Unix()),
		},
	).Result()
	if err != nil {
		s.T().Fatalf("Failed to populate db: %v", err)
	}

	s.client = client

	s.rd = redis.NewRedis(connStr, false, clock, logger)
	s.rd.Start()
}

func (s *RedisSuite) TearDownTest() {
	s.rd.Stop()
}

func TestRunRedisSuite(t *testing.T) {
	suite.Run(t, new(RedisSuite))
}

func (s *RedisSuite) TestMovingAverage() {
	value, err := s.rd.MovingAverage(
		1.5,
		"abc123",
		12,
		uint32(900),
	)
	assert.Nil(s.T(), err)
	assert.Equal(s.T(), 4.25, value)

	// we should still have 4 members of the set, i.e. the oldest has been deleted
	count, err := s.client.ZCard("abc123:12:900").Result()
	assert.Nil(s.T(), err)
	assert.Equal(s.T(), int64(4), count)

	// verify that we've deleted the out of range value from the set by checking
	// that the lowest score element in the set is now the second value we inserted
	result, err := s.client.ZPopMin("abc123:12:900", 1).Result()
	assert.Nil(s.T(), err)
	assert.Len(s.T(), result, 1)
	elem := strings.Split(result[0].Member.(string), ":")
	assert.Equal(s.T(), "5.5", elem[0])
}
