ALTER TABLE devices
  RENAME COLUMN topic TO device_token;

ALTER TABLE devices
  RENAME COLUMN disposition TO exposure;

ALTER TABLE devices
  DROP COLUMN private_key,
  DROP COLUMN user_uid;

ALTER TABLE streams
  ADD COLUMN token BYTEA NOT NULL,
  ADD COLUMN policy_id TEXT NOT NULL;