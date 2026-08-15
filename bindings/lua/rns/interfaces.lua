-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2024-2026 Quad4.io

local ffi_mod = require("rns.ffi")
local errors = require("rns.errors")
local ffi = ffi_mod.ffi
local lib = ffi_mod.lib

local function interfaces_list(node, capacity)
	capacity = capacity or 32
	if capacity <= 0 then
		errors.raise(errors.Error.INVALID_ARG)
	end
	local raw = ffi.new("rns_interface_entry[?]", capacity)
	local written = ffi.new("size_t[1]", 0)
	local code = lib.rns_interfaces(node:handle(), raw, capacity, written)
	if code ~= errors.Error.OK and code ~= errors.Error.TRUNCATED then
		errors.map_code(code)
	end
	local out = {}
	for i = 0, tonumber(written[0]) - 1 do
		local e = raw[i]
		out[#out + 1] = {
			name = ffi_mod.cstr(e.name, 96),
			type_name = ffi_mod.cstr(e.type_name, 32),
			online = e.online ~= 0,
			enabled = e.enabled ~= 0,
			rx_bytes = tonumber(e.rx_bytes),
			tx_bytes = tonumber(e.tx_bytes),
			rx_packets = tonumber(e.rx_packets),
			tx_packets = tonumber(e.tx_packets),
		}
	end
	return out
end

return {
	interfaces_list = interfaces_list,
}
