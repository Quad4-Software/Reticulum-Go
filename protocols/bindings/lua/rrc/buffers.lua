-- SPDX-License-Identifier: LicenseRef-Reticulum

local ffi = require("ffi")

local M = {}

M.MAX_BUFFER = 16 * 1024 * 1024
M.RRC_ERR_TRUNCATED = 8

function M.read_string(fill, initial)
	local cap = initial or 1024
	while cap <= M.MAX_BUFFER do
		local buf = ffi.new("char[?]", cap)
		local written = ffi.new("size_t[1]")
		local code = fill(buf, cap, written)
		if code == M.RRC_ERR_TRUNCATED then
			cap = cap * 2
		else
			return code, ffi.string(buf, written[0])
		end
	end
	return M.RRC_ERR_TRUNCATED, nil
end

function M.write_bytes(fill, initial)
	local cap = initial or 65536
	while cap <= M.MAX_BUFFER do
		local buf = ffi.new("uint8_t[?]", cap)
		local written = ffi.new("size_t[1]")
		local code = fill(buf, cap, written)
		if code == M.RRC_ERR_TRUNCATED then
			cap = cap * 2
		else
			return code, ffi.string(buf, written[0])
		end
	end
	return M.RRC_ERR_TRUNCATED, nil
end

return M
