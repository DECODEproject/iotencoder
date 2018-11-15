ALTER TABLE devices
  RENAME COLUMN device_token TO topic;

ALTER TABLE devices
  RENAME COLUMN exposure TO disposition;

ALTER TABLE devices
  ADD COLUMN private_key BYTEA NOT NULL,
  ADD COLUMN user_uid TEXT NOT NULL;

ALTER TABLE streams
  DROP COLUMN token,
  DROP COLUMN policy_id;