-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2024-2026 Quad4.io
--
-- NomadNet-style pageserver over LuaJIT librns bindings.
-- Usage: luajit main.lua -c config [-i identity] [-a announce_sec] [-p page_file]

local rns = require("rns")

local DEFAULT_ANNOUNCE_SEC = 900
local DEFAULT_PAGE_PATH = "/page/index.mu"
local DEFAULT_FILE_PATH = "/file/test.txt"
local DEFAULT_PAGE_FILE = "pages/index.mu"
local DEFAULT_FILE_FILE = "files/test.txt"
local DEFAULT_IDENTITY_PATH = "identity"
local REQ_DATA_CAP = 64 * 1024

local FALLBACK_PAGE = table.concat({
	"> Lua pageserver\n\n",
	"librns via Reticulum-Go\n\n",
	"Fallback page (file not found).\n\n",
})

local FALLBACK_FILE = "Test file from Reticulum-Go node!\n"

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

local function load_bytes(path, fallback)
	local f = io.open(path, "rb")
	if not f then
		io.stderr:write("warning: could not read " .. path .. ", using built-in content\n")
		return fallback
	end
	local data = f:read("*a")
	f:close()
	return data
end

local function load_or_create_identity(path)
	local ok, identity = pcall(rns.Identity.load, path)
	if ok then
		io.stderr:write("loaded identity from " .. path .. "\n")
		return identity
	end
	identity = rns.Identity.generate()
	identity:save(path)
	io.stderr:write("created and saved identity to " .. path .. "\n")
	return identity
end

local function parse_args(argv)
	local config_path
	local identity_path = DEFAULT_IDENTITY_PATH
	local announce_sec = DEFAULT_ANNOUNCE_SEC
	local page_file = DEFAULT_PAGE_FILE
	local file_file = DEFAULT_FILE_FILE
	local request_path = DEFAULT_PAGE_PATH
	local i = 1
	while i <= #argv do
		local a = argv[i]
		if a == "-c" then
			i = i + 1
			config_path = argv[i]
		elseif a == "-i" then
			i = i + 1
			identity_path = argv[i]
		elseif a == "-a" then
			i = i + 1
			announce_sec = tonumber(argv[i]) or -1
		elseif a == "-p" then
			i = i + 1
			page_file = argv[i]
		elseif a == "-f" then
			i = i + 1
			file_file = argv[i]
		elseif a == "-P" then
			i = i + 1
			request_path = argv[i]
		else
			die("unexpected argument: " .. a)
		end
		i = i + 1
	end
	if not config_path then
		die("usage: luajit main.lua -c config [options]")
	end
	if announce_sec < 0 then
		die("announce interval must be >= 0")
	end
	return config_path, identity_path, announce_sec, page_file, file_file, request_path
end

local config_path, identity_path, announce_sec, page_file, file_file, request_path = parse_args(arg)

local ver = rns.version()
if ver ~= rns.API_VERSION then
	die("librns version mismatch: got " .. ver)
end

local page_body = load_bytes(page_file, FALLBACK_PAGE)
local file_body = load_bytes(file_file, FALLBACK_FILE)

local node = rns.Node.create(config_path)
local identity = load_or_create_identity(identity_path)
node:set_identity(identity)
node:start()

local dest = rns.Destination.create(node, nil, "nomadnetwork", { "node" }, true)
dest:register_request_handler(request_path)
dest:register_request_handler(DEFAULT_FILE_PATH)

local dest_hex = rns.hash_to_hex(dest:hash())
print("DEST_HASH=" .. dest_hex)
print("REQUEST_PATH=" .. request_path)
print("FILE_PATH=" .. DEFAULT_FILE_PATH)
io.stderr:write("librns " .. ver .. " pageserver listening as nomadnetwork.node\n")
io.stderr:write("announce name=librns-lua-pageserver interval=" .. announce_sec .. "s\n")

local app_data = "librns-lua-pageserver"
if not pcall(function()
	dest:announce(app_data)
end) then
	print_last_error("destination.announce failed")
else
	io.stderr:write("announce sent\n")
end

local last_announce = os.time()
while true do
	if announce_sec > 0 and os.time() - last_announce >= announce_sec then
		pcall(function()
			dest:announce(app_data)
		end)
		io.stderr:write("announce refreshed\n")
		last_announce = os.time()
	end

	local ok_ev, ev_or_err = pcall(rns.Event.poll, node, 200, REQ_DATA_CAP)
	if not ok_ev then
		if type(ev_or_err) ~= "table" or ev_or_err.code ~= rns.Error.TIMEOUT then
			print_last_error("Event.poll failed")
			dest:close()
			identity:close()
			node:close()
			os.exit(1)
		end
	else
		local ev = ev_or_err
		local kind = ev:kind()
		if kind == rns.EventKind.LINK_ESTABLISHED then
			io.stderr:write("inbound link established\n")
		elseif kind == rns.EventKind.LINK_CLOSED then
			io.stderr:write("link closed\n")
		elseif kind == rns.EventKind.REQUEST_INCOMING then
			local path = ev:path()
			io.stderr:write("request incoming path=" .. path .. "\n")
			local req_id = ev:request_id()
			if path == request_path then
				if not pcall(rns.request_respond, node, req_id, page_body) then
					print_last_error("request_respond failed")
				else
					io.stderr:write("served " .. request_path .. " (" .. #page_body .. " bytes)\n")
				end
			elseif path == DEFAULT_FILE_PATH then
				if not pcall(rns.request_respond_file, node, req_id, "test.txt", file_body) then
					print_last_error("request_respond_file failed")
				else
					io.stderr:write("served " .. DEFAULT_FILE_PATH .. " (" .. #file_body .. " bytes)\n")
				end
			else
				pcall(rns.request_respond, node, req_id, "page not found\n")
			end
		end
	end
end
