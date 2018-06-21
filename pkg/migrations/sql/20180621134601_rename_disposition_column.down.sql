ALTER TYPE exposure RENAME TO disposition;

ALTER TABLE devices
  RENAME COLUMN exposure TO disposition;