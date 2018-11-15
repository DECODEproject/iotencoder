-- Decryption script for DECODE IoT Pilot

-- data schemas
keys_schema = SCHEMA.Record {
  community_seckey = SCHEMA.String
}

data_schema = SCHEMA.Record {
  header   = SCHEMA.String,
  encoding = SCHEMA.String,
  text     = SCHEMA.String,
  curve    = SCHEMA.String,
  zenroom  = SCHEMA.String,
  checksum = SCHEMA.String,
  iv       = SCHEMA.String
}

-- read and validate data
keys = read_json(KEYS, keys_schema)
data = read_json(DATA, data_schema)

header = MSG.unpack(base64(data.header):str())

community_key = ECDH.new()
community_key:private(base64(keys.community_seckey))

payload, ck = ECDH.decrypt(
  community_key,
  base64(header.device_pubkey),
  map(data, base64)
)

print(JSON.encode(MSG.unpack(payload.text:str())))