package redis

import (
	"fmt"
	"strconv"
	"time"

	kitlog "github.com/go-kit/kit/log"
	rd "github.com/go-redis/redis"
	"github.com/pkg/errors"
	"github.com/vmihailenco/msgpack"
)

// Clock is a local interface for some type that can return the current time
type Clock interface {
	Now() time.Time
}

type clock struct{}

// Now is our implementation of the Clock interface - just returns value of
// time.Now()
func (c clock) Now() time.Time {
	return time.Now()
}

// NewClock returns a valid implementation of our Clock interface
func NewClock() Clock {
	return clock{}
}

// Redis is our type that wraps the redis client and exposes an API to the rest
// of the application.
type Redis struct {
	connStr string
	verbose bool
	logger  kitlog.Logger
	client  *rd.Client
	clock   Clock
}

// NewRedis returns a new redis client instance
func NewRedis(connStr string, verbose bool, clock Clock, logger kitlog.Logger) *Redis {
	logger = kitlog.With(logger, "module", "redis")

	return &Redis{
		connStr: connStr,
		verbose: verbose,
		logger:  logger,
		clock:   clock,
	}
}

// Start starts the redis client, verifying that we can connect to redis
func (r *Redis) Start() error {
	r.logger.Log("msg", "starting redis client")

	opt, err := rd.ParseURL(r.connStr)
	if err != nil {
		return errors.Wrap(err, "failed to parse redis connection url")
	}

	client := rd.NewClient(opt)
	_, err = client.Ping().Result()
	if err != nil {
		return errors.Wrap(err, "failed to ping redis")
	}

	r.client = client

	return nil
}

// Stop the redis client
func (r *Redis) Stop() error {
	r.logger.Log("msg", "stopping redis client")
	return r.client.Close()
}

// Member is a type used for serializing unique values to redis. We must include
// a timestamp with each value as a sorted set will never store multiple entries
// with the same value (it is a set after all). However in order to calculate
// the mean we need to be able to count all values whether duplicate or now, so
// we build this object that includes the timestamp.
type Member struct {
	Timestamp int64   `msgpack:"timestamp"`
	Value     float64 `msgpack:"value"`
}

// MovingAverage is our main public method of the instance that calculates a
// moving average for the given value. Uses a Redis sorted set under the hood to
// maintain the running state of the average.
func (r *Redis) MovingAverage(value float64, deviceToken string, sensorID int, interval uint32) (float64, error) {
	key := BuildKey(deviceToken, sensorID, interval)

	if r.verbose {
		r.logger.Log("key", key, "msg", "calculating moving average")
	}

	now := r.clock.Now()
	intervalDuration := time.Second * time.Duration(-int(interval))
	previousTime := now.Add(intervalDuration)

	m := Member{
		Timestamp: now.Unix(),
		Value:     value,
	}

	b, err := msgpack.Marshal(m)
	if err != nil {
		return 0, errors.Wrap(err, "failed to marshal member to messagepack")
	}

	script := `
	-- read parameters
	local key = KEYS[1]
	local current_time = tonumber(ARGV[1])
	local previous_time = tonumber(ARGV[2])
	local value = ARGV[3]

	-- add the new value to the sorted set with score of current_time
	redis.call('zadd', key, current_time, value)

	-- get list of previous scores
	local values = redis.call('ZRANGEBYSCORE', key, previous_time, current_time)

	-- delete any value older than the previous time
	redis.call('ZREMRANGEBYSCORE', key, '-inf', previous_time)

	local acc = 0
	local counter = 0

	for i=1, #values do
		local v = cmsgpack.unpack(values[i])
		acc = acc + tonumber(v['value'])
		counter = counter + 1
	end

	return tostring(acc/counter)
	`

	avg, err := r.client.Eval(script, []string{key}, now.Unix(), previousTime.Unix(), b).Result()
	if err != nil {
		return 0, errors.Wrap(err, "failed to execute moving average script")
	}

	numericAvg, err := strconv.ParseFloat(avg.(string), 64)
	if err != nil {
		return 0, errors.Wrap(err, "failed to parse average value read from sorted set")
	}

	return numericAvg, nil
}

// Ping attempts to send a ping message to Redis, returning an error if we are
// unable to connect.
func (r *Redis) Ping() error {
	_, err := r.client.Ping().Result()
	if err != nil {
		return err
	}
	return nil
}

// BuildKey generates a key we will use for our sorted set we will use to
// calculate moving averages.
func BuildKey(deviceToken string, sensorID int, interval uint32) string {
	return fmt.Sprintf("%s:%v:%v", deviceToken, sensorID, interval)
}
