#!/usr/bin/env luajit
-- SPDX-License-Identifier: LicenseRef-Reticulum

package.path = "./?.lua;" .. package.path

local rrc = require("rrc")

if rrc.version() ~= rrc.API_VERSION then
	io.stderr:write("version mismatch\n")
	os.exit(1)
end

local node = rrc.node_create("")
rrc.node_start(node)
rrc.node_stop(node)
rrc.node_destroy(node)

local id = rrc.identity_generate()
if #rrc.identity_hash(id) ~= rrc.HASH_LEN then
	io.stderr:write("bad identity hash\n")
	os.exit(1)
end
rrc.identity_destroy(id)

local sender = string.char(0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15)
local env = rrc.envelope_create(rrc.RRC_TYPE_MSG, sender)
rrc.envelope_set_room(env, "lobby")
rrc.envelope_set_body_text(env, "hello")
local data = rrc.envelope_marshal(env)
rrc.envelope_destroy(env)

local got = rrc.envelope_unmarshal(data)
if rrc.envelope_body_text(got) ~= "hello" then
	io.stderr:write("body mismatch\n")
	os.exit(1)
end
rrc.envelope_destroy(got)

if rrc.normalize_room("  #Lobby ") ~= "#lobby" then
	io.stderr:write("normalize room failed\n")
	os.exit(1)
end

print("lua-smoke ok")
