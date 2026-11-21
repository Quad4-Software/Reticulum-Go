-- SPDX-License-Identifier: LicenseRef-Reticulum

local ffi_mod = require("rrc.ffi")
local buffers = require("rrc.buffers")
local lib = ffi_mod.lib
local ffi = require("ffi")

local M = {}

M.API_VERSION = "1.0"
M.HASH_LEN = ffi_mod.HASH_LEN
M.RRC_TYPE_MSG = ffi_mod.RRC_TYPE_MSG
M.DEFAULT_TIMEOUT_MS = 45000
M.RRC_EV_JOINED = 2
M.RRC_EV_MSG = 4

function M.version()
	local raw = lib.rrc_version()
	if raw == nil then
		return ""
	end
	return ffi.string(raw)
end

function M.last_error()
	local buf = ffi.new("char[?]", 512)
	local written = ffi.new("size_t[1]")
	local code = lib.rrc_last_error(buf, 512, written)
	local msg = ffi.string(buf, written[0])
	if msg and msg ~= "" then
		return msg
	end
	if code == ffi_mod.RRC_ERR_TRUNCATED then
		return "truncated"
	end
	return ""
end

function M.check(code)
	if code ~= ffi_mod.RRC_OK then
		error(M.last_error() ~= "" and M.last_error() or ("rrc error " .. tostring(code)), 2)
	end
end

function M.normalize_room(name)
	local code, out = buffers.read_string(function(buf, cap, written)
		return lib.rrc_normalize_room(name, buf, cap, written)
	end, 128)
	M.check(code)
	return out
end

function M.sanitize_nick(nick)
	local code, out = buffers.read_string(function(buf, cap, written)
		return lib.rrc_sanitize_nick(nick, buf, cap, written)
	end, 64)
	M.check(code)
	return out
end

function M.envelope_create(msg_type, sender)
	if #sender ~= M.HASH_LEN then
		error("sender hash length", 2)
	end
	local arr = ffi.new("uint8_t[?]", M.HASH_LEN, sender)
	local h = lib.rrc_envelope_create(msg_type, arr, M.HASH_LEN)
	if h == 0 then
		error("envelope create failed", 2)
	end
	return h
end

function M.envelope_destroy(h)
	if h ~= 0 then
		lib.rrc_envelope_destroy(h)
	end
end

function M.envelope_set_room(h, room)
	M.check(lib.rrc_envelope_set_room(h, room))
end

function M.envelope_set_nick(h, nick)
	M.check(lib.rrc_envelope_set_nick(h, nick))
end

function M.envelope_set_body_text(h, text)
	M.check(lib.rrc_envelope_set_body_text(h, text))
end

function M.envelope_set_destination(h, dest)
	if #dest ~= M.HASH_LEN then
		error("destination hash length", 2)
	end
	local arr = ffi.new("uint8_t[?]", M.HASH_LEN, dest)
	M.check(lib.rrc_envelope_set_destination(h, arr, M.HASH_LEN))
end

function M.envelope_body_text(h)
	local code, out = buffers.read_string(function(buf, cap, written)
		return lib.rrc_envelope_get_body_text(h, buf, cap, written)
	end, 1024)
	M.check(code)
	return out
end

function M.envelope_marshal(h)
	local code, out = buffers.write_bytes(function(buf, cap, written)
		return lib.rrc_envelope_marshal(h, buf, cap, written)
	end, 65536)
	M.check(code)
	return out
end

function M.envelope_unmarshal(data)
	local arr = ffi.new("uint8_t[?]", #data, data)
	local h = lib.rrc_envelope_unmarshal(arr, #data)
	if h == 0 then
		error("envelope unmarshal failed", 2)
	end
	return h
end

function M.node_create(config_path)
	local h = lib.rrc_node_create(config_path or "")
	if h == 0 then
		error("node create failed", 2)
	end
	return h
end

function M.node_destroy(h)
	if h ~= 0 then
		lib.rrc_node_destroy(h)
	end
end

function M.node_start(h)
	M.check(lib.rrc_node_start(h))
end

function M.node_stop(h)
	M.check(lib.rrc_node_stop(h))
end

function M.node_set_identity(h, identity)
	M.check(lib.rrc_node_set_identity(h, identity))
end

function M.node_add_udp_interface(h, name, local_addr, peer_addr)
	M.check(lib.rrc_node_add_udp_interface(h, name, local_addr, peer_addr))
end

function M.node_has_path(h, dest_hash)
	if #dest_hash ~= M.HASH_LEN then
		error("dest hash length", 2)
	end
	local arr = ffi.new("uint8_t[?]", M.HASH_LEN, dest_hash)
	local out = ffi.new("int[1]")
	M.check(lib.rrc_node_has_path(h, arr, M.HASH_LEN, out))
	return out[0] ~= 0
end

function M.identity_generate()
	local h = lib.rrc_identity_generate()
	if h == 0 then
		error("identity generate failed", 2)
	end
	return h
end

function M.identity_load(path)
	local h = lib.rrc_identity_load(path)
	if h == 0 then
		error("identity load failed", 2)
	end
	return h
end

function M.identity_save(h, path)
	M.check(lib.rrc_identity_save(h, path))
end

function M.identity_destroy(h)
	if h ~= 0 then
		lib.rrc_identity_destroy(h)
	end
end

function M.identity_hash(h)
	local buf = ffi.new("uint8_t[?]", M.HASH_LEN)
	local written = ffi.new("size_t[1]")
	M.check(lib.rrc_identity_hash(h, buf, M.HASH_LEN, written))
	return ffi.string(buf, written[0])
end

function M.identity_seed_destination(h, dest_hash)
	if #dest_hash ~= M.HASH_LEN then
		error("dest hash length", 2)
	end
	local arr = ffi.new("uint8_t[?]", M.HASH_LEN, dest_hash)
	M.check(lib.rrc_identity_seed_destination(h, arr, M.HASH_LEN))
end

function M.hub_create(node, identity, name, version)
	local h = lib.rrc_hub_create(node, identity, name, version)
	if h == 0 then
		error("hub create failed", 2)
	end
	return h
end

function M.hub_destroy(h)
	if h ~= 0 then
		lib.rrc_hub_destroy(h)
	end
end

function M.hub_start(h)
	M.check(lib.rrc_hub_start(h))
end

function M.hub_announce(h)
	M.check(lib.rrc_hub_announce(h))
end

function M.hub_hash(h)
	local buf = ffi.new("uint8_t[?]", M.HASH_LEN)
	local written = ffi.new("size_t[1]")
	M.check(lib.rrc_hub_hash(h, buf, M.HASH_LEN, written))
	return ffi.string(buf, written[0])
end

function M.hub_peer_count(h)
	local count = ffi.new("size_t[1]")
	M.check(lib.rrc_hub_peer_count(h, count))
	return tonumber(count[0])
end

local function slice_bytes(arr, len)
	len = tonumber(len) or 0
	local out = {}
	for i = 0, len - 1 do
		out[#out + 1] = string.char(arr[i])
	end
	return table.concat(out)
end

function M.poll_event(raw)
	return {
		kind = raw.kind,
		sender = slice_bytes(raw.sender, raw.sender_len),
		peer = slice_bytes(raw.peer, raw.peer_len),
		room = ffi.string(raw.room),
		nick = ffi.string(raw.nick),
		body = ffi.string(raw.body),
		msg_type = tonumber(raw.msg_type),
		room_truncated = raw.room_truncated ~= 0,
		nick_truncated = raw.nick_truncated ~= 0,
		body_truncated = raw.body_truncated ~= 0,
	}
end

function M.hub_event_poll(h, timeout_ms)
	local ev = ffi.new("rrc_event")
	local code = lib.rrc_hub_event_poll(h, timeout_ms or 0, ev)
	if code == ffi_mod.RRC_ERR_TIMEOUT then
		return nil, "timeout"
	end
	M.check(code)
	return M.poll_event(ev)
end

function M.client_dial(node, identity, hub_hash, nick, name, version, timeout_ms)
	if #hub_hash ~= M.HASH_LEN then
		error("hub hash length", 2)
	end
	local arr = ffi.new("uint8_t[?]", M.HASH_LEN, hub_hash)
	local h = lib.rrc_client_dial(
		node,
		identity,
		arr,
		M.HASH_LEN,
		nick,
		name,
		version,
		timeout_ms or M.DEFAULT_TIMEOUT_MS
	)
	if h == 0 then
		error("client dial failed", 2)
	end
	return h
end

function M.client_join(h, room)
	M.check(lib.rrc_client_join(h, room))
end

function M.client_part(h, room)
	M.check(lib.rrc_client_part(h, room))
end

function M.client_send_msg(h, room, text)
	M.check(lib.rrc_client_send_msg(h, room, text))
end

function M.client_send_notice(h, room, text)
	M.check(lib.rrc_client_send_notice(h, room, text))
end

function M.client_send_action(h, room, text)
	M.check(lib.rrc_client_send_action(h, room, text))
end

function M.client_ping(h)
	M.check(lib.rrc_client_ping(h))
end

function M.client_close(h)
	if h ~= 0 then
		lib.rrc_client_close(h)
	end
end

function M.client_event_poll(h, timeout_ms)
	local ev = ffi.new("rrc_event")
	local code = lib.rrc_client_event_poll(h, timeout_ms or 0, ev)
	if code == ffi_mod.RRC_ERR_TIMEOUT then
		return nil, "timeout"
	end
	M.check(code)
	return M.poll_event(ev)
end

function M.sleep_ms(ms)
	local deadline = os.clock() + (ms / 1000)
	while os.clock() < deadline do
	end
end

return M
