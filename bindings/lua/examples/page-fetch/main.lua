-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2024-2026 Quad4.io
--
-- NomadNet-style page fetch over LuaJIT librns bindings.
-- Usage: luajit main.lua -c config [-t timeout_sec] <dest_hash>:<page_path>

local rns = require("rns")

local PAGE_BUF_CAP = 512 * 1024
local DEFAULT_TIMEOUT_SEC = 60
local PATH_RETRY_SEC = 2

local function die(msg)
	io.stderr:write(msg .. "\n")
	os.exit(1)
end

local function print_last_error(what)
	local msg = rns.last_error()
	if msg ~= "" then
		io.stderr:write(what .. ": " .. msg .. "\n")
	else
		io.stderr:write(what .. "\n")
	end
end

local function parse_args(argv)
	local config_path, target
	local timeout_sec = DEFAULT_TIMEOUT_SEC
	local i = 1
	while i <= #argv do
		local a = argv[i]
		if a == "-c" then
			i = i + 1
			config_path = argv[i]
		elseif a == "-t" then
			i = i + 1
			timeout_sec = tonumber(argv[i]) or 0
		elseif not target then
			target = a
		else
			die("unexpected argument: " .. a)
		end
		i = i + 1
	end
	if not config_path or not target then
		die("usage: luajit main.lua -c config [-t timeout] <dest_hash>:<page_path>")
	end
	if timeout_sec <= 0 then
		die("timeout must be positive")
	end
	return config_path, target, timeout_sec
end

local function parse_target(target)
	local hex_part, page_path = target:match("^([^:]+):(.+)$")
	if not hex_part or not page_path then
		error("bad target", 0)
	end
	return rns.hex_to_hash(hex_part), page_path
end

local config_path, target, timeout_sec = parse_args(arg)

local ver = rns.version()
if ver ~= rns.API_VERSION then
	die("librns version mismatch: got " .. ver)
end

local dest_hash, page_path = parse_target(target)
local dest_hex = rns.hash_to_hex(dest_hash)

local node = rns.Node.create(config_path)
local identity = rns.Identity.generate()
node:set_identity(identity)
node:start()

print(string.format("librns %s fetching %s from %s", ver, page_path, dest_hex))

local deadline = os.time() + timeout_sec
local last_path_req = 0
local need_path_req = true
local saw_announce = false
local link = nil

local function cleanup()
	if link then
		link:close()
	end
	identity:close()
	node:close()
end

while os.time() < deadline and link == nil do
	local t = os.time()
	if need_path_req or t - last_path_req >= PATH_RETRY_SEC then
		local ok = pcall(rns.path_request, node, dest_hash)
		if not ok then
			print_last_error("path_request failed")
		end
		last_path_req = t
		need_path_req = false
		if rns.path_known(node, dest_hash) then
			io.stderr:write("path known, waiting for destination identity announce\n")
		else
			io.stderr:write("requesting path to " .. dest_hex .. "\n")
		end
	end

	local ok_ev, ev_or_err = pcall(rns.Event.poll, node, 200, PAGE_BUF_CAP)
	if not ok_ev then
		if type(ev_or_err) == "table" and ev_or_err.code == rns.Error.TIMEOUT then
			if saw_announce or rns.path_known(node, dest_hash) then
				local ok_link, opened = pcall(rns.Link.open, node, dest_hash)
				if ok_link then
					link = opened
				end
			end
		else
			print_last_error("Event.poll failed")
			cleanup()
			os.exit(1)
		end
	else
		local ev = ev_or_err
		if ev:kind() == rns.EventKind.ANNOUNCE and rns.hash_eq(ev:destination_hash(), dest_hash) then
			saw_announce = true
			io.stderr:write("announce for target (hops=" .. ev:hops() .. ")\n")
			local ok_link, opened = pcall(rns.Link.open, node, dest_hash)
			if ok_link then
				link = opened
			else
				print_last_error("Link.open after announce")
			end
		elseif ev:kind() == rns.EventKind.LINK_FAILED then
			io.stderr:write("link failed while opening: " .. ev:error_message() .. "\n")
		end
	end
end

if link == nil then
	cleanup()
	die("timed out before link open")
end

local established = false
while os.time() < deadline and not established do
	local ok_ev, ev_or_err = pcall(rns.Event.poll, node, 500, PAGE_BUF_CAP)
	if not ok_ev then
		if type(ev_or_err) ~= "table" or ev_or_err.code ~= rns.Error.TIMEOUT then
			print_last_error("Event.poll failed")
			cleanup()
			os.exit(1)
		end
	else
		local ev = ev_or_err
		if ev:kind() == rns.EventKind.LINK_ESTABLISHED then
			established = true
			io.stderr:write("link established\n")
		elseif ev:kind() == rns.EventKind.LINK_FAILED then
			cleanup()
			die("link establishment failed: " .. ev:error_message())
		elseif ev:kind() == rns.EventKind.LINK_CLOSED then
			cleanup()
			die("link closed before establish")
		end
	end
end

if not established then
	cleanup()
	die("timed out waiting for link establishment")
end

local remaining_ms = math.max((deadline - os.time()) * 1000, 1000)
if not pcall(function()
	link:request(node, page_path, "", remaining_ms)
end) then
	print_last_error("link.request failed")
	cleanup()
	os.exit(1)
end
io.stderr:write("request sent for " .. page_path .. "\n")

while os.time() < deadline do
	local ok_ev, ev_or_err = pcall(rns.Event.poll, node, 500, PAGE_BUF_CAP)
	if not ok_ev then
		if type(ev_or_err) ~= "table" or ev_or_err.code ~= rns.Error.TIMEOUT then
			print_last_error("Event.poll failed")
			cleanup()
			os.exit(1)
		end
	else
		local ev = ev_or_err
		if ev:kind() == rns.EventKind.REQUEST_RESPONSE then
			local data = ev:app_data()
			print(string.format("\n=== Page Content (%d bytes) ===", #data))
			io.write(data)
			if #data == 0 or data:sub(-1) ~= "\n" then
				io.write("\n")
			end
			if ev:app_data_truncated() then
				io.stderr:write("warning: response truncated\n")
			end
			print("=== End of Page ===")
			cleanup()
			os.exit(0)
		elseif ev:kind() == rns.EventKind.REQUEST_FAILED then
			cleanup()
			die("request failed: " .. ev:error_message())
		elseif ev:kind() == rns.EventKind.LINK_CLOSED then
			cleanup()
			die("link closed before response")
		end
	end
end

cleanup()
die("timed out waiting for page response")
