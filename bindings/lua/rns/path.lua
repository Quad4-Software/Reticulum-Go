-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2024-2026 Quad4.io

local ffi_mod = require("rns.ffi")
local errors = require("rns.errors")
local ffi = ffi_mod.ffi
local lib = ffi_mod.lib

local function path_request(node, dest_hash)
	if type(dest_hash) ~= "string" or #dest_hash ~= ffi_mod.HASH_LEN then
		errors.raise(errors.Error.INVALID_ARG)
	end
	local buf = ffi.new("uint8_t[?]", ffi_mod.HASH_LEN)
	ffi.copy(buf, dest_hash, ffi_mod.HASH_LEN)
	errors.map_code(lib.rns_path_request(node:handle(), buf))
end

local function path_table(node, capacity, max_hops)
	capacity = capacity or 256
	max_hops = max_hops or -1
	if capacity <= 0 then
		errors.raise(errors.Error.INVALID_ARG)
	end
	local raw = ffi.new("rns_path_entry[?]", capacity)
	local written = ffi.new("size_t[1]", 0)
	local code = lib.rns_path_table(node:handle(), raw, capacity, written, max_hops)
	if code ~= errors.Error.OK and code ~= errors.Error.TRUNCATED then
		errors.map_code(code)
	end
	local out = {}
	for i = 0, tonumber(written[0]) - 1 do
		local e = raw[i]
		out[#out + 1] = {
			hash = ffi.string(e.hash, e.hash_len),
			via = ffi.string(e.via, e.via_len),
			hops = tonumber(e.hops),
			iface = ffi_mod.cstr(e.iface, 64),
			timestamp = tonumber(e.timestamp),
			expires = tonumber(e.expires),
		}
	end
	return out
end

local function path_known(node, dest_hash)
	if type(dest_hash) ~= "string" or #dest_hash ~= ffi_mod.HASH_LEN then
		return false
	end
	local ok, table = pcall(path_table, node)
	if not ok then
		return false
	end
	for _, entry in ipairs(table) do
		if entry.hash == dest_hash then
			return true
		end
	end
	return false
end

return {
	path_request = path_request,
	path_table = path_table,
	path_known = path_known,
}
