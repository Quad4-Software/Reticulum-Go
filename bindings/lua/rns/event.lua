-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2024-2026 Quad4.io

local ffi_mod = require("rns.ffi")
local errors = require("rns.errors")
local ffi = ffi_mod.ffi
local lib = ffi_mod.lib

local EventKind = {
	NONE = 0,
	ANNOUNCE = 1,
	LINK_ESTABLISHED = 2,
	LINK_FAILED = 3,
	LINK_DATA = 4,
	LINK_CLOSED = 5,
	REQUEST_INCOMING = 6,
	REQUEST_RESPONSE = 7,
	REQUEST_FAILED = 8,
	RESOURCE_STARTED = 9,
	RESOURCE_CONCLUDED = 10,
	DESTINATION_DATA = 11,
}

local Event = {}
Event.__index = Event

function Event.poll(node, timeout_ms, app_data_cap)
	local event = ffi.new("rns_event")
	local buf = nil
	local cap = app_data_cap or 0
	if cap > 0 then
		buf = ffi.new("uint8_t[?]", cap)
		event.app_data = buf
		event.app_data_cap = cap
	end
	errors.map_code(lib.rns_event_poll(node:handle(), event, timeout_ms or 0))
	local data = ""
	if event.app_data ~= nil and event.app_data_len > 0 then
		data = ffi.string(event.app_data, event.app_data_len)
	end
	return setmetatable({ _raw = event, _app_data = data, _keep = buf }, Event)
end

function Event:kind()
	return tonumber(self._raw.kind)
end

function Event:hops()
	return tonumber(self._raw.hops)
end

function Event:link_id()
	return ffi.string(self._raw.link_id, self._raw.link_id_len)
end

function Event:destination_hash()
	return ffi.string(self._raw.destination_hash, self._raw.destination_hash_len)
end

function Event:identity_hash()
	return ffi.string(self._raw.identity_hash, self._raw.identity_hash_len)
end

function Event:request_id()
	return ffi.string(self._raw.request_id, self._raw.request_id_len)
end

function Event:path()
	return ffi_mod.cstr(self._raw.path, 256)
end

function Event:error_message()
	return ffi_mod.cstr(self._raw.error_message, 256)
end

function Event:app_data()
	return self._app_data
end

function Event:app_data_truncated()
	return self._raw.app_data_truncated ~= 0
end

local function hash_eq(a, b)
	return type(a) == "string" and type(b) == "string"
		and #a == ffi_mod.HASH_LEN and #b == ffi_mod.HASH_LEN and a == b
end

return {
	Event = Event,
	EventKind = EventKind,
	hash_eq = hash_eq,
}
