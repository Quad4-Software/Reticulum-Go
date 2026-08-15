-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2024-2026 Quad4.io

local ffi_mod = require("rns.ffi")
local lib = ffi_mod.lib
local ffi = ffi_mod.ffi

local Error = {
	OK = 0,
	INVALID_ARG = 1,
	INVALID_HANDLE = 2,
	NOT_FOUND = 3,
	STATE = 4,
	IO = 5,
	INTERNAL = 6,
	TIMEOUT = 7,
	TRUNCATED = 8,
}

local names = {
	[0] = "ok",
	[1] = "invalid argument",
	[2] = "invalid handle",
	[3] = "not found",
	[4] = "invalid state",
	[5] = "io error",
	[6] = "internal error",
	[7] = "timeout",
	[8] = "truncated",
}

local function name(code)
	return names[code] or "internal error"
end

local function raise(code, message)
	error({ code = code, message = message or name(code) }, 2)
end

local function map_code(code)
	if code ~= Error.OK then
		raise(code)
	end
end

local function version()
	local raw = lib.rns_version()
	if raw == nil then
		return ""
	end
	return ffi.string(raw)
end

local function last_error()
	local buf = ffi.new("char[512]")
	local written = ffi.new("size_t[1]", 0)
	local code = lib.rns_last_error(buf, 512, written)
	if code ~= Error.OK or written[0] == 0 then
		return ""
	end
	return ffi.string(buf, written[0])
end

return {
	Error = Error,
	name = name,
	raise = raise,
	map_code = map_code,
	version = version,
	last_error = last_error,
}
