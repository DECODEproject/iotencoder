-- File: encrypt.lua
-- Script Params:
--  DATA and KEYS has to be passed to this script
--  through zenroom
-- Returns:
--  Nothing just print to stdout encrypted DATA
octet = require 'octet'
ecdh = require 'ecdh'
json = require 'json'

msg = octet.new(#DATA)
msg:string(DATA)

keys = json.decode(KEYS)
keyring = ecdh.new('ec25519')

public = octet.new()
public:base64(keys.public)

private = octet.new()
private:base64(keys.private)
keyring:public(public)
keyring:private(private)

sess = keyring:session(public)
zmsg = keyring:encrypt(sess, msg):base64()
print(zmsg)