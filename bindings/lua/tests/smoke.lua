-- SPDX-License-Identifier: Apache-2.0
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

print("lua smoke tests ok")
