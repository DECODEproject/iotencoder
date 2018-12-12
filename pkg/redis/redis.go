package redis

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	kitlog "github.com/go-kit/kit/log"
	rd "github.com/go-redis/redis"
	"github.com/pkg/errors"
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

	logger.Log("msg", "creating redis client")

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

// MovingAverage is our main public method of the instance that calculates a
// moving average for the given value. Uses a Redis sorted set under the hood to
// maintain the running state of the average.
func (r *Redis) MovingAverage(value float64, deviceToken string, sensorID int, interval uint32) (float64, error) {
	key := BuildKey(deviceToken, sensorID, interval)

	now := r.clock.Now()
	intervalDuration := time.Second * time.Duration(-int(interval))
	previousTime := now.Add(intervalDuration)

	_, err := r.client.ZAdd(key, rd.Z{
		Score:  float64(now.Unix()),
		Member: fmt.Sprintf("%v:%v", value, now.Unix()),
	}).Result()

	if err != nil {
		return 0, errors.Wrap(err, "failed to add value to sorted set")
	}

	vals, err := r.client.ZRangeByScore(key, rd.ZRangeBy{
		Min: strconv.FormatInt(previousTime.Unix(), 10),
		Max: strconv.FormatInt(now.Unix(), 10),
	}).Result()
	if err != nil {
		return 0, errors.Wrap(err, "failed to read values from sorted set")
	}

	_, err = r.client.ZRemRangeByScore(
		key,
		"-inf",
		strconv.FormatInt(previousTime.Unix(), 10),
	).Result()
	if err != nil {
		return 0, errors.Wrap(err, "failed to delete old values from sorted set")
	}

	return CalculateAverage(vals)
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

// CalculateAverage is the stateless function that calculates a simple average
// for the given list of values. Redis returns values as strings, so we need to
// convert before calculating.
func CalculateAverage(vals []string) (float64, error) {
	if len(vals) == 0 {
		return 0, nil
	}

	var acc float64

	for _, val := range vals {
		parts := strings.Split(val, ":")
		if len(parts) != 2 {
			return 0, errors.New("invalid value pulled from sorted set")
		}

		numericVal, err := strconv.ParseFloat(parts[0], 64)
		if err != nil {
			return 0, errors.Wrap(err, "failed to parse float value read from sorted set")
		}
		acc = acc + numericVal
	}

	return acc / float64(len(vals)), nil
}
