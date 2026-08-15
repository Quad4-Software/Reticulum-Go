-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2024-2026 Quad4.io

local ffi_mod = require("rns.ffi")
local errors = require("rns.errors")
local ffi = ffi_mod.ffi
local lib = ffi_mod.lib

local Destination = {}
Destination.__index = Destination

local Handle = ffi.typeof("struct { uint64_t h; }")

local function wrap(handle)
	local box = ffi.new(Handle)
	box.h = handle
	local self = setmetatable({ _box = box }, Destination)
	ffi.gc(box, function(b)
		if b.h ~= 0 then
			lib.rns_destination_destroy(b.h)
			b.h = 0
		end
	end)
	return self
end

function Destination.create(node, identity, app_name, aspects, accepts_links)
	if not app_name or app_name == "" then
		errors.raise(errors.Error.INVALID_ARG)
	end
	aspects = aspects or {}
	local n = #aspects
	local aspect_ptrs = nil
	if n > 0 then
		aspect_ptrs = ffi.new("const char*[?]", n)
		for i = 1, n do
			aspect_ptrs[i - 1] = aspects[i]
		end
	end
	local id_handle = identity and identity:handle() or 0
	local h = lib.rns_destination_create(
		node:handle(),
		id_handle,
		app_name,
		aspect_ptrs,
		n,
		accepts_links and 1 or 0
	)
	if h == 0 then
		errors.raise(errors.Error.INTERNAL)
	end
	return wrap(h)
end

function Destination:enable_ratchets(path)
	local p = nil
	if path and path ~= "" then
		p = path
	end
	errors.map_code(lib.rns_destination_enable_ratchets(self._box.h, p))
end

function Destination:enforce_ratchets()
	errors.map_code(lib.rns_destination_enforce_ratchets(self._box.h))
end

function Destination:announce(app_data)
	local buf, n = ffi_mod.to_uint8(app_data or "")
	errors.map_code(lib.rns_destination_announce(self._box.h, buf, n))
end

function Destination:hash()
	local out = ffi.new("uint8_t[?]", ffi_mod.HASH_LEN)
	local written = ffi.new("size_t[1]", 0)
	errors.map_code(lib.rns_destination_hash(self._box.h, out, ffi_mod.HASH_LEN, written))
	if written[0] ~= ffi_mod.HASH_LEN then
		errors.raise(errors.Error.TRUNCATED)
	end
	return ffi.string(out, written[0])
end

function Destination:register_request_handler(path)
	if not path or path == "" then
		errors.raise(errors.Error.INVALID_ARG)
	end
	errors.map_code(lib.rns_destination_register_request_handler(self._box.h, path))
end

function Destination:handle()
	return self._box.h
end

function Destination:close()
	if self._box.h ~= 0 then
		lib.rns_destination_destroy(self._box.h)
		self._box.h = 0
	end
end

return Destination
