# Aggregation Algorithm and Implementation

## Status

Proposed

## Context

It is a requirement of the project that the stream encoder provide as a one
of its building blocks the ability to apply optional filtering or aggregation
primitives to data before writing it to the datastore.

It has been agreed with the consortium that we will implement the following
three sharing options for each sensor:

- SHARE - i.e. share the sensor without any modification, just write the raw
  data directly to the datastore
- BINNING - allow the policy to specify a list of bins for a sensor, into which
  received values should be categorised.
- MOVING AVERAGE - allow the policy to specify a time window for which we will
  calculate a moving average for received values.

The purpose of this document is to specify a method by which we will
calculate moving averages for received data.

Moving averages are allowed to be specified for individual sensors with
different time intervals, meaning that a single policy might specify a 5
minute moving average for noise levels, but a 30 minute moving average for
light levels, and the system should be able to handle this.

### Incoming data

The data received from SmartCitizen has the following structure:

```json
{
  "data": [
    {
      "recorded_at": "2018-06-03T22:26:02Z",
      "sensors": [
        {
          "id": 10,
          "value": 100
        },
        {
          "id": 29,
          "value": 67.02939
        },
        {
          "id": 13,
          "value": 74.65033
        },
        {
          "id": 12,
          "value": 22.41268
        },
        {
          "id": 14,
          "value": 5.29416
        }
      ]
    }
  ]
}
```

### Redis

> "Redis is an open source (BSD licensed), in-memory data structure store,
> used as a database, cache and message broker."

It provides a number of interesting data structures from a simple key/values,
to much more complex structures (hyperloglog, geospatial indexes, etc.),
however the structure I am interested in for the purposes of this document
are a type called a sorted set.

A sorted set in Redis is a data type with the following characteristics: it
is a set (in the mathematical sense), so a given key, i.e. the name of the
set, points to a collection of unique elements. The difference between a
sorted set and a normal set however, is that as well as the value which we
are storing, we also give each element a float score. This means that the
unique element in the set is the combination of a score and a value. If we
try and insert a new element to the sorted set with the identical score and
value of an existing element it will be a no-op.

For example we can create a new sorted set by simply calling:

```
ZADD team_members 1974 "Sam Mulube"
```

Where the above inserts a new entry into the team_members set with a score of
1974, and it's crucial to note here that the score can be any numerical
value, where here I've arbitrarily chosen to use my year of birth. This fact
will be important later, so just remember it for now.

Another entry can be added as follows:

```
ZADD team_members 1969 "Andrew Chetty"
```

Once values have been inserted to the set there are various operations that
allow data to be retrieved, modified and deleted using the scores to
efficiently manage this.

## Decision

We will use Redis, and specifically sorted sets within Redis in order to
implement simple moving averages for received data.

### Algorithm

1.  Every time an event is received for a device, we will look up all of the
    streams associated with that device.

2.  From this collection of streams we will obtain a unique list comprising
    `device_token`, `sensorId` pairs for every sensorId that declares a type of
    moving average.

3.  These elements will be used to create sorted set keys of the form:
    `deviceTopic:sensorId`.

4.  We calculate a "score" for the currently received value using epoch time
    in seconds expressed as a float.

5.  For each `deviceToken:sensorId` pair we write the value into a sorted set,
    using the timestamp as the score.

6.  We use `ZRANGEBYSCORE` for each entitlement where the score range is
    calculated based on the diff between the "current timestamp" and the
    timestamp X seconds ago in order to calculate the average for that time
    window. This value is then emitted to the datastore.

7.  We find the longest interval for this device, and use `ZREMRANGEBYSCORE`
    to delete values longer than the longest possible interval to prevent the
    storage from expanding infinitely.

## Consequences

- Lot of work is pushed into redis to maintain state for averages, and this is
  not the most efficient of algorithms
- Adds a temporary store of data at the encoder level, which if compromised
  would be a problem. We could look at encrypting this data.
