-- Encryption script for DECODE IoT Pilot
curve = 'ed25519'

-- data schema to validate input
keys_schema = SCHEMA.Record {
  device_token     = SCHEMA.String,
  community_id     = SCHEMA.String,
  community_pubkey = SCHEMA.String
}

-- import and validate KEYS data
keys = read_json(KEYS, keys_schema)

-- generate a new device keypair every time
device_key = ECDH.keygen(curve)

-- read the payload we will encrypt
payload = {}
payload['device_token'] = keys['device_token']
payload['data'] = DATA

-- The device's public key, community_id and the curve type are tranmitted in
-- clear inside the header, which is authenticated AEAD
header = {}
header['device_pubkey'] = device_key:public():base64()
header['community_id'] = keys['community_id']

iv = RNG.new():octet(16)
header['iv'] = iv:base64()

-- encrypt the data, and build our output object
local session = device_key:session(base64(keys.community_pubkey))
local head = str(MSG.pack(header))
local out = { header = head }
out.text, out.checksum = ECDH.aead_encrypt(session, str(MSG.pack(payload)), iv, head)

output = map(out, base64)
output.zenroom = VERSION
output.encoding = 'base64'
output.curve = curve

print(JSON.encode(output))