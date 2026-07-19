-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2024-2026 Quad4.io

local ffi_mod = require("rns.ffi")
local errors = require("rns.errors")
local ffi = ffi_mod.ffi
local lib = ffi_mod.lib

local Identity = {}
Identity.__index = Identity

local Handle = ffi.typeof("struct { uint64_t h; }")

local function wrap(handle)
	local box = ffi.new(Handle)
	box.h = handle
	local self = setmetatable({ _box = box }, Identity)
	ffi.gc(box, function(b)
		if b.h ~= 0 then
			lib.rns_identity_destroy(b.h)
			b.h = 0
		end
	end)
	return self
end

function Identity.generate()
	local h = lib.rns_identity_generate()
	if h == 0 then
		errors.raise(errors.Error.INTERNAL)
	end
	return wrap(h)
end

function Identity.load(path)
	if not path or path == "" then
		errors.raise(errors.Error.INVALID_ARG)
	end
	local h = lib.rns_identity_load(path)
	if h == 0 then
		errors.raise(errors.Error.IO)
	end
	return wrap(h)
end

function Identity.from_public_key(pub)
	local buf, n = ffi_mod.to_uint8(pub)
	if n == 0 then
		errors.raise(errors.Error.INVALID_ARG)
	end
	local h = lib.rns_identity_from_public_key(buf, n)
	if h == 0 then
		errors.raise(errors.Error.INVALID_ARG)
	end
	return wrap(h)
end

function Identity:save(path)
	if not path or path == "" then
		errors.raise(errors.Error.INVALID_ARG)
	end
	errors.map_code(lib.rns_identity_save(self._box.h, path))
end

function Identity:hash_hex()
	local buf = ffi.new("char[64]")
	local written = ffi.new("size_t[1]", 0)
	errors.map_code(lib.rns_identity_hash(self._box.h, buf, 64, written))
	return ffi.string(buf, written[0])
end

function Identity:hash_bytes()
	local out = ffi.new("uint8_t[?]", ffi_mod.HASH_LEN)
	local written = ffi.new("size_t[1]", 0)
	errors.map_code(lib.rns_identity_hash_bytes(self._box.h, out, ffi_mod.HASH_LEN, written))
	if written[0] ~= ffi_mod.HASH_LEN then
		errors.raise(errors.Error.TRUNCATED)
	end
	return ffi.string(out, written[0])
end

function Identity:public_key()
	local out = ffi.new("uint8_t[64]")
	local written = ffi.new("size_t[1]", 0)
	errors.map_code(lib.rns_identity_public_key(self._box.h, out, 64, written))
	return ffi.string(out, written[0])
end

function Identity:sign(data)
	local buf, n = ffi_mod.to_uint8(data)
	local sig = ffi.new("uint8_t[64]")
	local written = ffi.new("size_t[1]", 0)
	errors.map_code(lib.rns_identity_sign(self._box.h, buf, n, sig, 64, written))
	return ffi.string(sig, written[0])
end

function Identity:verify(data, signature)
	local dbuf, dn = ffi_mod.to_uint8(data)
	local sbuf, sn = ffi_mod.to_uint8(signature)
	errors.map_code(lib.rns_identity_verify(self._box.h, dbuf, dn, sbuf, sn))
end

function Identity:handle()
	return self._box.h
end

function Identity:close()
	if self._box.h ~= 0 then
		lib.rns_identity_destroy(self._box.h)
		self._box.h = 0
	end
end

return Identity
