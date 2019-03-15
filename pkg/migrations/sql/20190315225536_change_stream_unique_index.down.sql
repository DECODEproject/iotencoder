DROP INDEX IF EXISTS streams_device_id_community_id_idx;

CREATE UNIQUE INDEX IF NOT EXISTS streams_device_id_public_key_idx
  ON streams(device_id, public_key);