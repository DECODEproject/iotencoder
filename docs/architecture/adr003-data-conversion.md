ADR003 - Data conversions

## Status

Proposed

## Context

We receive minimal data from SmartCitizen via MQTT which we need to process
and emit in encrypted form to the datastore such that consumers can usefully
use the data if granted access. To this end we must process the data to add
in metadata that we know (like the location and exposure), but also reformat
the data after applying the processing filters to the data.

### Input Event Format

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

## Decision

We will generate output data in the format shown below. We show output in the
context of four specific policies we wish to illustrate.

### Policy 1

Emit just 15 minute moving average for noise level (29)

Emitted json

```json
{
  "data": [
    {
      "location": {
        "longitude": 2.256,
        "latitude": 49.253
      },
      "exposure": "OUTDOOR",
      "recorded_at": "2018-06-03T22:26:02Z",
      "sensors": [
        {
          "id": 29,
          "name": "MEMS Mic",
          "description":
            "MEMS microphone with enveolope follower sound pressure sensor (noise)",
          "unit": "dBC",
          "type": "MOVING_AVG",
          "interval": 900,
          "value": 64.252
        }
      ]
    }
  ]
}
```

### Policy 2

Emit binned noise levels, below 40 dBC, between 80 and 40 dBC, and above 80 dBC.

Emitted json:

```json
{
  "data": [
    {
      "location": {
        "longitude": 2.256,
        "latitude": 49.253
      },
      "exposure": "OUTDOOR",
      "recorded_at: "2018-06-03T22:26:02Z",
      "sensors": [
        {
          "id": 29
          "name": "MEMS Mic",
          "description":
            "MEMS microphone with enveolope follower sound pressure sensor (noise)",
          "unit": "dBC",
          "type": "BIN",
          "bins": [40, 80],
          "values": [0, 1, 0]
        }
      ]
    }
  ]
}
```

### Policy 3

Emit binned noise levels, below or above 40 dBC, and 15 minute moving average light levels.

Emitted json:

```json
{
  "data": [
    {
      "location": {
        "longitude": 2.256,
        "latitude": 49.253
      },
      "exposure": "OUTDOOR",
      "recorded_at: "2018-06-03T22:26:02Z",
      "sensors": [
        {
          "id": 14,
          "name": "BH1730FVC",
          "description": "Digital Ambient Light Sensor",
          "unit": "lux",
          "type": "MOVING_AVG",
          "interval": 900,
          "value": 6.34
        },
        {
          "id": 29
          "name": "MEMS Mic",
          "description":
            "MEMS microphone with enveolope follower sound pressure sensor (noise)",
          "unit": "dBC",
          "type": "BIN",
          "bins": [40],
          "values": [0, 1]
        }
      ]
    }
  ]
}
```

### Policy 4

Emit full resolution temperature data.

Emitted json:

```json
{
  "data": [
    {
      "location": {
        "longitude": 2.256,
        "latitude": 49.253
      },
      "exposure": "OUTDOOR",
      "recorded_at: "2018-06-03T22:26:02Z",
      "sensors": [
        {
          "id": 12,
          "name": "HPP828E031",
          "description": "Temperature"
          "unit": "ºC",
          "type": "SHARE",
          "value": 22.41268
        }
      ]
    }
  ]
}
```

## Consequences

- We must have some way of adding extra metadata to each event without pinging
  SmartCitizen for every incoming request.

-
