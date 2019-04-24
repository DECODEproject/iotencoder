-- Decryption script for DECODE IoT Pilot

-- curve used
curve = 'ed25519'

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
  checksum = SCHEMA.String
}

-- read and validate data
keys = read_json(KEYS, keys_schema)
data = read_json(DATA, data_schema)

header = MSG.unpack(base64(data.header):str())

community_key = ECDH.new(curve)
community_key:private(base64(keys.community_seckey))

session = community_key:session(base64(header.device_pubkey))

decode = { header = header }
decode.text, decode.checksum = ECDH.aead_decrypt(session, base64(data.text), base64(header.iv), base64(data.header))

print(JSON.encode(MSG.unpack(decode.text:str())))