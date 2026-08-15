-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2024-2026 Quad4.io

local ffi_mod = require("rns.ffi")
local errors = require("rns.errors")
local ffi = ffi_mod.ffi
local lib = ffi_mod.lib

local function rsg_create(identity, message, embed)
	local msg, n = ffi_mod.to_uint8(message or "")
	local needed = ffi.new("size_t[1]", 0)
	local code = lib.rns_rsg_create(identity:handle(), msg, n, embed and 1 or 0, nil, 0, needed)
	if code ~= errors.Error.OK and code ~= errors.Error.TRUNCATED then
		errors.map_code(code)
	end
	if needed[0] == 0 then
		errors.raise(errors.Error.INTERNAL)
	end
	local out = ffi.new("uint8_t[?]", needed[0])
	local written = ffi.new("size_t[1]", 0)
	errors.map_code(lib.rns_rsg_create(identity:handle(), msg, n, embed and 1 or 0, out, needed[0], written))
	return ffi.string(out, written[0])
end

local function rsg_validate(rsg, message, required_signer_hash)
	local rbuf, rn = ffi_mod.to_uint8(rsg)
	local mbuf, mn = ffi_mod.to_uint8(message or "")
	local sbuf, sn = ffi_mod.to_uint8(required_signer_hash or "")
	errors.map_code(lib.rns_rsg_validate(rbuf, rn, mbuf, mn, sbuf, sn))
end

local function rsm_verify(rsm, required_signer_hash)
	local rbuf, rn = ffi_mod.to_uint8(rsm)
	local sbuf, sn = ffi_mod.to_uint8(required_signer_hash or "")
	local needed = ffi.new("size_t[1]", 0)
	local code = lib.rns_rsm_verify(rbuf, rn, sbuf, sn, nil, 0, needed)
	if code ~= errors.Error.OK and code ~= errors.Error.TRUNCATED then
		errors.map_code(code)
	end
	if needed[0] == 0 then
		return ""
	end
	local out = ffi.new("uint8_t[?]", needed[0])
	local written = ffi.new("size_t[1]", 0)
	errors.map_code(lib.rns_rsm_verify(rbuf, rn, sbuf, sn, out, needed[0], written))
	return ffi.string(out, written[0])
end

return {
	rsg_create = rsg_create,
	rsg_validate = rsg_validate,
	rsm_verify = rsm_verify,
}
