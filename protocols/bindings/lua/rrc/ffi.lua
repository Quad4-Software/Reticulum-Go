-- SPDX-License-Identifier: LicenseRef-Reticulum

local ffi = require("ffi")

local HASH_LEN = 16

ffi.cdef[[
typedef struct rrc_event {
	int kind;
	uint8_t sender[16];
	size_t sender_len;
	uint8_t peer[16];
	size_t peer_len;
	char room[128];
	int room_truncated;
	char nick[64];
	int nick_truncated;
	char body[1024];
	int body_truncated;
	uint64_t msg_type;
} rrc_event;

const char *rrc_version(void);
int rrc_last_error(char *buf, size_t buf_len, size_t *written);

uint64_t rrc_envelope_create(uint64_t msg_type, const uint8_t *sender, size_t sender_len);
int rrc_envelope_set_room(uint64_t envelope, const char *room);
int rrc_envelope_set_nick(uint64_t envelope, const char *nick);
int rrc_envelope_set_body_text(uint64_t envelope, const char *text);
int rrc_envelope_set_destination(uint64_t envelope, const uint8_t *dest, size_t dest_len);
int rrc_envelope_get_type(uint64_t envelope, uint64_t *out);
int rrc_envelope_get_sender(uint64_t envelope, uint8_t *out, size_t out_len, size_t *written);
int rrc_envelope_get_room(uint64_t envelope, char *buf, size_t buf_len, size_t *written);
int rrc_envelope_get_nick(uint64_t envelope, char *buf, size_t buf_len, size_t *written);
int rrc_envelope_get_body_text(uint64_t envelope, char *buf, size_t buf_len, size_t *written);
int rrc_envelope_marshal(uint64_t envelope, uint8_t *out, size_t out_len, size_t *written);
uint64_t rrc_envelope_unmarshal(const uint8_t *data, size_t data_len);
int rrc_envelope_destroy(uint64_t envelope);

int rrc_normalize_room(const char *in, char *out, size_t out_len, size_t *written);
int rrc_sanitize_nick(const char *in, char *out, size_t out_len, size_t *written);

uint64_t rrc_node_create(const char *config_path);
int rrc_node_start(uint64_t node);
int rrc_node_stop(uint64_t node);
int rrc_node_destroy(uint64_t node);
int rrc_node_set_identity(uint64_t node, uint64_t identity);
int rrc_node_add_udp_interface(uint64_t node, const char *name, const char *local_addr, const char *peer_addr);
int rrc_node_has_path(uint64_t node, const uint8_t *dest_hash, size_t dest_hash_len, int *has_path);

uint64_t rrc_identity_generate(void);
uint64_t rrc_identity_load(const char *path);
int rrc_identity_save(uint64_t identity, const char *path);
int rrc_identity_destroy(uint64_t identity);
int rrc_identity_hash(uint64_t identity, uint8_t *out, size_t out_len, size_t *written);
int rrc_identity_seed_destination(uint64_t identity, const uint8_t *dest_hash, size_t dest_hash_len);

uint64_t rrc_hub_create(uint64_t node, uint64_t identity, const char *name, const char *version);
int rrc_hub_start(uint64_t hub);
int rrc_hub_announce(uint64_t hub);
int rrc_hub_hash(uint64_t hub, uint8_t *out, size_t out_len, size_t *written);
int rrc_hub_peer_count(uint64_t hub, size_t *count);
int rrc_hub_destroy(uint64_t hub);
int rrc_hub_event_poll(uint64_t hub, int timeout_ms, rrc_event *event);

uint64_t rrc_client_dial(uint64_t node, uint64_t identity, const uint8_t *hub_hash, size_t hub_hash_len,
	const char *nick, const char *name, const char *version, int timeout_ms);
int rrc_client_join(uint64_t client, const char *room);
int rrc_client_part(uint64_t client, const char *room);
int rrc_client_send_msg(uint64_t client, const char *room, const char *text);
int rrc_client_send_notice(uint64_t client, const char *room, const char *text);
int rrc_client_send_action(uint64_t client, const char *room, const char *text);
int rrc_client_ping(uint64_t client);
int rrc_client_close(uint64_t client);
int rrc_client_event_poll(uint64_t client, int timeout_ms, rrc_event *event);
]]

local function lib_names()
	if ffi.os == "OSX" then
		return { "librrc.dylib" }
	end
	if ffi.os == "Windows" then
		return { "rrc.dll", "librrc.dll" }
	end
	return { "librrc.so" }
end

local function candidates()
	local out = {}
	local env = os.getenv("RRC_LIB_PATH")
	if env and env ~= "" then
		out[#out + 1] = env
	end
	local root = os.getenv("RRC_ROOT")
	if root and root ~= "" then
		for _, name in ipairs(lib_names()) do
			out[#out + 1] = root .. "/bin/" .. name
		end
	end
	for _, name in ipairs(lib_names()) do
		out[#out + 1] = "../../bin/" .. name
		out[#out + 1] = "../../../bin/" .. name
	end
	return out
end

local function load_lib()
	for _, path in ipairs(candidates()) do
		local ok, lib = pcall(ffi.load, path)
		if ok then
			return lib
		end
	end
	return ffi.load("rrc")
end

local lib = load_lib()

return {
	HASH_LEN = HASH_LEN,
	RRC_OK = 0,
	RRC_ERR_TIMEOUT = 7,
	RRC_ERR_TRUNCATED = 8,
	RRC_TYPE_MSG = 20,
	lib = lib,
}
