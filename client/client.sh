#!/usr/bin/env bash

set -euo pipefail

# This script attempts to invoke the three RPC methods exposed by the datastore
# via Curl just for quick local sanity checking. Requires base64, curl and jq
# tools installed and available on the current $PATH to run.

hello=$(echo hello | base64)
world=$(echo world | base64)

echo "--> write some data for alice for public key abc123"
curl --request "POST" \
     --location "http://localhost:8080/twirp/datastore.Datastore/WriteData" \
     --header "Content-Type: application/json" \
     --silent \
     --data "{\"public_key\":\"abc123\",\"user_uid\":\"alice\",\"data\":\"$hello\"}" \
     | jq "."

curl --request "POST" \
     --location "http://localhost:8080/twirp/datastore.Datastore/WriteData" \
     --header "Content-Type: application/json" \
     --silent \
     --data "{\"public_key\":\"abc123\",\"user_uid\":\"alice\",\"data\":\"$world\"}" \
     | jq "."

echo "--> write some data for bob for public key abc123"
curl --request "POST" \
     --location "http://localhost:8080/twirp/datastore.Datastore/WriteData" \
     --header "Content-Type: application/json" \
     --silent \
     --data "{\"public_key\":\"abc123\",\"user_uid\":\"bob\",\"data\":\"$hello\"}" \
     | jq "."

curl --request "POST" \
     --location "http://localhost:8080/twirp/datastore.Datastore/WriteData" \
     --header "Content-Type: application/json" \
     --silent \
     --data "{\"public_key\":\"abc123\",\"user_uid\":\"bob\",\"data\":\"$world\"}" \
     | jq "."

echo "--> read all data for public_key abc123"
curl --request "POST" \
     --location "http://localhost:8080/twirp/datastore.Datastore/ReadData" \
     --header "Content-Type: application/json" \
     --silent \
     --data '{"public_key":"abc123", "page_size":3}' \
     | jq "."

echo "--> capture next page cursor"
cursor=$(curl --request "POST" \
     --location "http://localhost:8080/twirp/datastore.Datastore/ReadData" \
     --header "Content-Type: application/json" \
     --silent \
     --data '{"public_key":"abc123", "page_size":3}' \
     | jq -r ".next_page_cursor")

echo "-- read next page of data for abc123"
curl --request "POST" \
     --location "http://localhost:8080/twirp/datastore.Datastore/ReadData" \
     --header "Content-Type: application/json" \
     --silent \
     --data "{\"public_key\":\"abc123\",\"page_size\":3,\"page_cursor\":\"$cursor\"}" \
     | jq "."

echo "--> delete alice's data"
curl --request "POST" \
     --location "http://localhost:8080/twirp/datastore.Datastore/DeleteData" \
     --header "Content-Type: application/json" \
     --silent \
     --data '{"user_uid":"alice"}' \
     | jq "."

echo "--> read all data for public_key abc123 again"
curl --request "POST" \
     --location "http://localhost:8080/twirp/datastore.Datastore/ReadData" \
     --header "Content-Type: application/json" \
     --silent \
     --data '{"public_key":"abc123"}' \
     | jq "."

echo "--> delete data for bob"
curl --request "POST" \
     --location "http://localhost:8080/twirp/datastore.Datastore/DeleteData" \
     --header "Content-Type: application/json" \
     --silent \
     --data '{"user_uid":"bob"}' \
     | jq "."
