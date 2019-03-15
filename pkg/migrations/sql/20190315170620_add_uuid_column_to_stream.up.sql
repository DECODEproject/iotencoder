ALTER TABLE streams
  ADD COLUMN uuid UUID NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS streams_uuid_idx
  ON streams (uuid);