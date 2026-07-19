-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2024-2026 Quad4.io

local ffi = require("ffi")

local HASH_LEN = 16

ffi.cdef[[
typedef struct rns_event {
	int kind;
	uint8_t link_id[16];
	size_t link_id_len;
	uint8_t destination_hash[16];
	size_t destination_hash_len;
	uint8_t identity_hash[16];
	size_t identity_hash_len;
	uint8_t request_id[16];
	size_t request_id_len;
	uint8_t hops;
	char path[256];
	int path_truncated;
	char error_message[256];
	int error_message_truncated;
	uint8_t *app_data;
	size_t app_data_len;
	size_t app_data_cap;
	int app_data_truncated;
} rns_event;

typedef struct rns_path_entry {
	uint8_t hash[16];
	size_t hash_len;
	uint8_t via[16];
	size_t via_len;
	uint8_t hops;
	char iface[64];
	double timestamp;
	double expires;
} rns_path_entry;

typedef struct rns_interface_entry {
	char name[96];
	char type_name[32];
	int online;
	int enabled;
	uint64_t rx_bytes;
	uint64_t tx_bytes;
	uint64_t rx_packets;
	uint64_t tx_packets;
} rns_interface_entry;

const char *rns_version(void);
int rns_last_error(char *buf, size_t buf_len, size_t *written);

uint64_t rns_node_create(const char *config_path);
int rns_node_start(uint64_t node);
int rns_node_stop(uint64_t node);
int rns_node_destroy(uint64_t node);
int rns_node_set_identity(uint64_t node, uint64_t identity);
int rns_node_resume(uint64_t node);
int rns_node_pause(uint64_t node);
int rns_node_refresh_paths(uint64_t node, const uint8_t *dest_hashes, size_t count);

uint64_t rns_identity_generate(void);
uint64_t rns_identity_load(const char *path);
int rns_identity_save(uint64_t identity, const char *path);
int rns_identity_destroy(uint64_t identity);
int rns_identity_hash(uint64_t identity, char *hex_buf, size_t hex_buf_len, size_t *written);
int rns_identity_hash_bytes(uint64_t identity, uint8_t *out, size_t out_len, size_t *written);
int rns_identity_public_key(uint64_t identity, uint8_t *out, size_t out_len, size_t *written);
uint64_t rns_identity_from_public_key(const uint8_t *pub, size_t pub_len);
int rns_identity_sign(uint64_t identity, const uint8_t *data, size_t data_len,
	uint8_t *sig_out, size_t sig_out_len, size_t *written);
int rns_identity_verify(uint64_t identity, const uint8_t *data, size_t data_len,
	const uint8_t *sig, size_t sig_len);

int rns_rsg_create(uint64_t identity, const uint8_t *message, size_t message_len, int embed,
	uint8_t *out, size_t out_len, size_t *written);
int rns_rsg_validate(const uint8_t *rsg, size_t rsg_len,
	const uint8_t *message, size_t message_len,
	const uint8_t *required_signer_hash, size_t required_signer_hash_len);
int rns_rsm_verify(const uint8_t *rsm, size_t rsm_len,
	const uint8_t *required_signer_hash, size_t required_signer_hash_len,
	uint8_t *message_out, size_t message_out_len, size_t *written);

uint64_t rns_destination_create(uint64_t node, uint64_t identity, const char *app_name,
	const char *const *aspects, size_t aspect_count, int accepts_links);
int rns_destination_announce(uint64_t destination, const uint8_t *app_data, size_t app_data_len);
int rns_destination_hash(uint64_t destination, uint8_t *hash_out, size_t hash_out_len, size_t *written);
int rns_destination_destroy(uint64_t destination);
int rns_destination_register_request_handler(uint64_t destination, const char *path);

int rns_path_request(uint64_t node, const uint8_t *dest_hash);
int rns_path_table(uint64_t node, rns_path_entry *out, size_t out_cap, size_t *written, int max_hops);
int rns_interfaces(uint64_t node, rns_interface_entry *out, size_t out_cap, size_t *written);

uint64_t rns_link_open(uint64_t node, const uint8_t *dest_hash);
int rns_link_send(uint64_t link, const uint8_t *data, size_t data_len);
int rns_link_send_resource(uint64_t link, const uint8_t *data, size_t data_len, const char *name);
int rns_link_close(uint64_t link);
int rns_link_id(uint64_t link, uint8_t *id_out, size_t id_out_len, size_t *written);
int rns_link_request(uint64_t node, uint64_t link, const char *path,
	const uint8_t *data, size_t data_len, int timeout_ms,
	uint8_t *request_id_out, size_t request_id_out_len, size_t *written);

int rns_request_respond(uint64_t node, const uint8_t *request_id, size_t request_id_len,
	const uint8_t *data, size_t data_len);
int rns_request_respond_file(uint64_t node, const uint8_t *request_id, size_t request_id_len,
	const char *filename, const uint8_t *data, size_t data_len);

int rns_event_poll(uint64_t node, rns_event *event, int timeout_ms);
]]

local function candidates()
	local out = {}
	local env = os.getenv("RNS_LIB_PATH")
	if env and env ~= "" then
		out[#out + 1] = env
	end
	local root = os.getenv("RNS_ROOT")
	if root and root ~= "" then
		out[#out + 1] = root .. "/bin/librns.so"
	end
	local info = debug.getinfo(1, "S")
	local src = info and info.source or ""
	if src:sub(1, 1) == "@" then
		local dir = src:sub(2):match("(.*/)")
		if dir then
			out[#out + 1] = dir .. "../../../bin/librns.so"
			out[#out + 1] = dir .. "../../bin/librns.so"
		end
	end
	out[#out + 1] = "bin/librns.so"
	out[#out + 1] = "../bin/librns.so"
	out[#out + 1] = "../../bin/librns.so"
	return out
end

local function file_exists(path)
	local f = io.open(path, "rb")
	if not f then
		return false
	end
	f:close()
	return true
end

local function load_library(path)
	if path then
		return ffi.load(path)
	end
	for _, candidate in ipairs(candidates()) do
		if file_exists(candidate) then
			return ffi.load(candidate)
		end
	end
	return ffi.load("rns")
end

local lib = load_library()

local function cstr(buf, max_len)
	local n = max_len or #buf
	for i = 0, n - 1 do
		if buf[i] == 0 then
			return ffi.string(buf, i)
		end
	end
	return ffi.string(buf, n)
end

local function bytes_from(ptr, len)
	if ptr == nil or len == 0 then
		return ""
	end
	return ffi.string(ptr, len)
end

local function to_uint8(data)
	if data == nil or data == "" then
		return nil, 0
	end
	local n = #data
	local buf = ffi.new("uint8_t[?]", n)
	ffi.copy(buf, data, n)
	return buf, n
end

return {
	ffi = ffi,
	lib = lib,
	HASH_LEN = HASH_LEN,
	cstr = cstr,
	bytes_from = bytes_from,
	to_uint8 = to_uint8,
	load_library = load_library,
}
