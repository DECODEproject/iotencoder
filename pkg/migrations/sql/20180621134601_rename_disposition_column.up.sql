ALTER TYPE disposition RENAME TO exposure;

ALTER TABLE devices
  RENAME COLUMN disposition TO exposure;