package redis_test

import (
	"os"
	"testing"
	"time"

	kitlog "github.com/go-kit/kit/log"
	rd "github.com/go-redis/redis"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"github.com/vmihailenco/msgpack"

	"github.com/DECODEproject/iotencoder/pkg/mocks"
	"github.com/DECODEproject/iotencoder/pkg/redis"
)

func TestBuildKey(t *testing.T) {
	key := redis.BuildKey("abc123", 12, uint32(300))
	assert.Equal(t, "abc123:12:300", key)
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
			Member: buildMember(s.T(), 4.5, firstTime),
		},
		rd.Z{
			Score:  float64(secondTime.Unix()),
			Member: buildMember(s.T(), 5.5, secondTime),
		},
		rd.Z{
			Score:  float64(thirdTime.Unix()),
			Member: buildMember(s.T(), 6.5, thirdTime),
		},
		rd.Z{
			Score:  float64(fourthTime.Unix()),
			Member: buildMember(s.T(), 5.5, fourthTime),
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
	assert.Equal(s.T(), 4.75, value)

	// we should still have 4 members of the set, i.e. the oldest has been deleted
	count, err := s.client.ZCard("abc123:12:900").Result()
	assert.Nil(s.T(), err)
	assert.Equal(s.T(), int64(4), count)

	// verify that we've deleted the out of range value from the set by checking
	// that the lowest score element in the set is now the second value we inserted
	result, err := s.client.ZPopMin("abc123:12:900", 1).Result()
	assert.Nil(s.T(), err)
	assert.Len(s.T(), result, 1)

	var m redis.Member
	err = msgpack.Unmarshal([]byte(result[0].Member.(string)), &m)
	assert.Nil(s.T(), err)
	assert.Equal(s.T(), 5.5, m.Value)
}

func buildMember(t *testing.T, val float64, timestamp time.Time) []byte {
	t.Helper()

	m := redis.Member{
		Timestamp: timestamp.Unix(),
		Value:     val,
	}

	b, err := msgpack.Marshal(m)
	if err != nil {
		t.Fatalf("failed to marshal msgpack: %v", err)
	}

	return b
}
