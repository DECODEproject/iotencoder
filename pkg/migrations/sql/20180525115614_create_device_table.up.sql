CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TYPE disposition AS ENUM ('indoor', 'outdoor');

CREATE TABLE IF NOT EXISTS devices (
  id SERIAL PRIMARY KEY,
  broker TEXT NOT NULL,
  topic TEXT NOT NULL,
  private_key BYTEA NOT NULL,
  user_uid TEXT NOT NULL,
  longitude DOUBLE PRECISION NOT NULL,
  latitude DOUBLE PRECISION NOT NULL,
  disposition disposition NOT NULL,
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS devices_topic_idx
  ON devices (topic);

CREATE INDEX IF NOT EXISTS devices_user_uid_idx
  ON devices (user_uid);