-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2024-2026 Quad4.io

local rns = require("rns")

local ver = rns.version()
if ver ~= rns.API_VERSION then
	io.stderr:write("unexpected version: " .. tostring(ver) .. "\n")
	os.exit(1)
end

local node = rns.Node.create()
local ok, err = pcall(function()
	node:start()
	local ok2, err2 = pcall(function()
		node:event_poll(10)
	end)
	if ok2 or not err2 or err2.code ~= rns.Error.TIMEOUT then
		error("expected timeout poll on idle node")
	end
	node:stop()
end)
node:close()
if not ok then
	io.stderr:write(tostring(err.message or err) .. "\n")
	os.exit(1)
end

print("lua-smoke ok")
