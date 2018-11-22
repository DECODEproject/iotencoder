CREATE TABLE IF NOT EXISTS streams (
  id SERIAL PRIMARY KEY,
  device_id INTEGER NOT NULL REFERENCES devices(id),
  public_key TEXT NOT NULL,
  policy_id TEXT NOT NULL,
  token BYTEA NOT NULL,
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS streams_device_id_public_key_idx
  ON streams(device_id, public_key);
