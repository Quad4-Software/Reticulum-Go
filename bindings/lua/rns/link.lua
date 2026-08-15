-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2024-2026 Quad4.io

local ffi_mod = require("rns.ffi")
local errors = require("rns.errors")
local ffi = ffi_mod.ffi
local lib = ffi_mod.lib

local Link = {}
Link.__index = Link

local Handle = ffi.typeof("struct { uint64_t h; }")

local function wrap(handle)
	local box = ffi.new(Handle)
	box.h = handle
	local self = setmetatable({ _box = box }, Link)
	ffi.gc(box, function(b)
		if b.h ~= 0 then
			lib.rns_link_close(b.h)
			b.h = 0
		end
	end)
	return self
end

function Link.open(node, dest_hash)
	if type(dest_hash) ~= "string" or #dest_hash ~= ffi_mod.HASH_LEN then
		errors.raise(errors.Error.INVALID_ARG)
	end
	local buf = ffi.new("uint8_t[?]", ffi_mod.HASH_LEN)
	ffi.copy(buf, dest_hash, ffi_mod.HASH_LEN)
	local h = lib.rns_link_open(node:handle(), buf)
	if h == 0 then
		errors.raise(errors.Error.INTERNAL)
	end
	return wrap(h)
end

function Link:send(data)
	local buf, n = ffi_mod.to_uint8(data)
	if n == 0 then
		errors.raise(errors.Error.INVALID_ARG)
	end
	errors.map_code(lib.rns_link_send(self._box.h, buf, n))
end

function Link:send_resource(data, name)
	local buf, n = ffi_mod.to_uint8(data or "")
	errors.map_code(lib.rns_link_send_resource(self._box.h, buf, n, name or nil))
end

function Link:id()
	local out = ffi.new("uint8_t[?]", ffi_mod.HASH_LEN)
	local written = ffi.new("size_t[1]", 0)
	errors.map_code(lib.rns_link_id(self._box.h, out, ffi_mod.HASH_LEN, written))
	if written[0] ~= ffi_mod.HASH_LEN then
		errors.raise(errors.Error.TRUNCATED)
	end
	return ffi.string(out, written[0])
end

function Link:request(node, path, data, timeout_ms)
	if not path or path == "" then
		errors.raise(errors.Error.INVALID_ARG)
	end
	local buf, n = ffi_mod.to_uint8(data or "")
	local request_id = ffi.new("uint8_t[?]", ffi_mod.HASH_LEN)
	local written = ffi.new("size_t[1]", 0)
	errors.map_code(lib.rns_link_request(
		node:handle(),
		self._box.h,
		path,
		buf,
		n,
		timeout_ms or 0,
		request_id,
		ffi_mod.HASH_LEN,
		written
	))
	if written[0] ~= ffi_mod.HASH_LEN then
		errors.raise(errors.Error.TRUNCATED)
	end
	return ffi.string(request_id, written[0])
end

function Link:handle()
	return self._box.h
end

function Link:close()
	if self._box.h ~= 0 then
		lib.rns_link_close(self._box.h)
		self._box.h = 0
	end
end

local function request_respond(node, request_id, data)
	local req, rn = ffi_mod.to_uint8(request_id)
	if rn == 0 then
		errors.raise(errors.Error.INVALID_ARG)
	end
	local buf, n = ffi_mod.to_uint8(data or "")
	errors.map_code(lib.rns_request_respond(node:handle(), req, rn, buf, n))
end

local function request_respond_file(node, request_id, filename, data)
	local req, rn = ffi_mod.to_uint8(request_id)
	if rn == 0 or not filename or filename == "" then
		errors.raise(errors.Error.INVALID_ARG)
	end
	local buf, n = ffi_mod.to_uint8(data or "")
	errors.map_code(lib.rns_request_respond_file(node:handle(), req, rn, filename, buf, n))
end

return {
	Link = Link,
	request_respond = request_respond,
	request_respond_file = request_respond_file,
}
