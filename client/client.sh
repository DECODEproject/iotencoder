#!/usr/bin/env bash

set -euo pipefail

# This script attempts to invoke the three RPC methods exposed by the datastore
# via Curl just for quick local sanity checking. Requires base64, curl and jq
# tools installed and available on the current $PATH to run.

echo "--> create a stream"
curl --request "POST" \
     --location "http://localhost:8080/twirp/encoder.Encoder/CreateStream" \
     --header "Content-Type: application/json" \
     --silent \
     --data '{"broker_address":"tcp://mqtt.smartcitizen.me:1883","device_topic":"device/sck/2047f0/readings","device_private_key":"abc123","recipient_public_key":"hij567","user_uid":"alice","location":{"longitude":0.023,"latitude":55.253},"disposition":"INDOOR"}' \
     | jq "."

sleep 5

curl --request "POST" \
     --location "http://localhost:8080/twirp/encoder.Encoder/CreateStream" \
     --header "Content-Type: application/json" \
     --silent \
     --data '{"broker_address":"tcp://mqtt.smartcitizen.me:1883","device_topic":"device/sck/f4d5fb/readings","device_private_key":"abc123","recipient_public_key":"hij567","user_uid":"alice","location":{"longitude":0.023,"latitude":55.253},"disposition":"INDOOR"}' \
     | jq "."
