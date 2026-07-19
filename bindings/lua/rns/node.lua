-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2024-2026 Quad4.io

local ffi_mod = require("rns.ffi")
local errors = require("rns.errors")
local ffi = ffi_mod.ffi
local lib = ffi_mod.lib

local Node = {}
Node.__index = Node

local Handle = ffi.typeof("struct { uint64_t h; }")

local function wrap(handle)
	local box = ffi.new(Handle)
	box.h = handle
	local self = setmetatable({ _box = box }, Node)
	ffi.gc(box, function(b)
		if b.h ~= 0 then
			lib.rns_node_destroy(b.h)
			b.h = 0
		end
	end)
	return self
end

function Node.create(config_path)
	local path = config_path or ""
	local h = lib.rns_node_create(path)
	if h == 0 then
		errors.raise(errors.Error.INTERNAL)
	end
	return wrap(h)
end

function Node:start()
	errors.map_code(lib.rns_node_start(self._box.h))
end

function Node:stop()
	errors.map_code(lib.rns_node_stop(self._box.h))
end

function Node:set_identity(identity)
	errors.map_code(lib.rns_node_set_identity(self._box.h, identity:handle()))
end

function Node:pause()
	errors.map_code(lib.rns_node_pause(self._box.h))
end

function Node:resume()
	errors.map_code(lib.rns_node_resume(self._box.h))
end

function Node:event_poll(timeout_ms, app_data_cap)
	local Event = require("rns.event")
	return Event.poll(self, timeout_ms, app_data_cap or 65536)
end

function Node:handle()
	return self._box.h
end

function Node:close()
	if self._box.h ~= 0 then
		lib.rns_node_destroy(self._box.h)
		self._box.h = 0
	end
end

return Node
