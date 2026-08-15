-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2024-2026 Quad4.io

local errors = require("rns.errors")

local hexmap = {
	[0] = "0", "1", "2", "3", "4", "5", "6", "7", "8", "9", "a", "b", "c", "d", "e", "f",
}

local function hash_to_hex(data)
	if type(data) ~= "string" or #data ~= 16 then
		errors.raise(errors.Error.INVALID_ARG)
	end
	local out = {}
	for i = 1, 16 do
		local b = data:byte(i)
		out[#out + 1] = hexmap[math.floor(b / 16)]
		out[#out + 1] = hexmap[b % 16]
	end
	return table.concat(out)
end

local function hex_to_hash(text)
	if type(text) ~= "string" or #text ~= 32 then
		errors.raise(errors.Error.INVALID_ARG)
	end
	local out = {}
	for i = 1, 32, 2 do
		local byte = tonumber(text:sub(i, i + 1), 16)
		if not byte then
			errors.raise(errors.Error.INVALID_ARG)
		end
		out[#out + 1] = string.char(byte)
	end
	return table.concat(out)
end

return {
	hash_to_hex = hash_to_hex,
	hex_to_hash = hex_to_hash,
}
