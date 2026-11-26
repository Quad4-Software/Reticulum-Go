-- SPDX-License-Identifier: LicenseRef-Reticulum
-- Copyright (c) 2024-2026 Quad4.io

local rns = require("rns")

local function assert_eq(a, b, msg)
	if a ~= b then
		error((msg or "assert_eq") .. ": got " .. tostring(a) .. " want " .. tostring(b), 2)
	end
end

local function assert_true(cond, msg)
	if not cond then
		error(msg or "assert_true failed", 2)
	end
end

assert_eq(rns.version(), rns.API_VERSION, "version")

do
	local node = rns.Node.create()
	local ok, err = pcall(function()
		node:start()
		local ok2, err2 = pcall(function()
			node:event_poll(10)
		end)
		assert_true(not ok2, "expected timeout")
		assert_eq(err2.code, rns.Error.TIMEOUT, "timeout code")
		node:stop()
	end)
	node:close()
	assert_true(ok, err and (err.message or tostring(err)) or "node lifecycle")
end

do
	local identity = rns.Identity.generate()
	local hex_hash = identity:hash_hex()
	assert_eq(#hex_hash, 32, "hash hex length")
	assert_eq(#identity:hash_bytes(), 16, "hash bytes length")
	local msg = "hello-rns"
	local sig = identity:sign(msg)
	identity:verify(msg, sig)
	local pub = identity:public_key()
	local only_pub = rns.Identity.from_public_key(pub)
	only_pub:verify(msg, sig)
	only_pub:close()
	identity:close()
end

do
	local node = rns.Node.create()
	local ok, err = pcall(function() node:reload_config() end)
	assert_true(not ok and err.code == rns.Error.INVALID_ARG, "reload without config")
	local config_path = os.tmpname()
	local cf = assert(io.open(config_path, "wb"))
	cf:write("[reticulum]\nenable_transport = no\nin_memory_storage = yes\n")
	cf:close()
	local configured = rns.Node.create(config_path)
	configured:reload_config()
	configured:close()
	os.remove(config_path)
	local unknown = string.rep("\0", 16)
	ok, err = pcall(function() rns.destination_encrypt(unknown, "x") end)
	assert_true(not ok and err.code == rns.Error.NOT_FOUND, "destination encrypt unknown")
	ok, err = pcall(function() node:packet_send(unknown, "x") end)
	assert_true(not ok and (err.code == rns.Error.STATE or err.code == rns.Error.NOT_FOUND), "packet send unknown")
	ok, err = pcall(function() rns.link_from_id(node, unknown) end)
	assert_true(not ok and err.code == rns.Error.NOT_FOUND, "link from id")
	node:set_event_callback(function(_ev) end)
	node:set_event_callback(nil)
	local identity = rns.Identity.generate()
	local dest = rns.Destination.create(node, identity, "luatest", {}, false)
	dest:set_proof_strategy(0)
	ok, err = pcall(function() dest:set_proof_strategy(255) end)
	assert_true(not ok and err.code == rns.Error.INVALID_ARG, "proof strategy")
	dest:close()
	local path = os.tmpname()
	local f = assert(io.open(path, "wb"))
	f:write("signed")
	f:close()
	local sig = rns.rsg_sign_file(identity, path)
	rns.rsg_verify_file(sig, path)
	f = assert(io.open(path, "ab"))
	f:write("x")
	f:close()
	ok, err = pcall(function() rns.rsg_verify_file(sig, path) end)
	assert_true(not ok, "tampered rsg")
	os.remove(path)
	identity:close()
	node:close()
end

print("lua smoke tests ok")
