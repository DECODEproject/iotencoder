CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TYPE exposure AS ENUM ('unknown', 'indoor', 'outdoor');

CREATE TABLE IF NOT EXISTS devices (
  id SERIAL PRIMARY KEY,
  broker TEXT NOT NULL,
  device_token TEXT NOT NULL,
  longitude DOUBLE PRECISION NOT NULL,
  latitude DOUBLE PRECISION NOT NULL,
  exposure exposure NOT NULL DEFAULT 'unknown'::exposure,
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS devices_token_idx
  ON devices (device_token);