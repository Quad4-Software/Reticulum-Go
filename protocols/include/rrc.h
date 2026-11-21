#ifndef RRC_H
#define RRC_H

#include <stddef.h>
#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

#define RRC_API_VERSION "1.0"

#define RRC_HASH_LEN 16
#define RRC_MSG_ID_LEN 8

#define RRC_OK 0
#define RRC_ERR_INVALID_ARG 1
#define RRC_ERR_INVALID_HANDLE 2
#define RRC_ERR_NOT_FOUND 3
#define RRC_ERR_STATE 4
#define RRC_ERR_IO 5
#define RRC_ERR_INTERNAL 6
#define RRC_ERR_TIMEOUT 7
#define RRC_ERR_TRUNCATED 8

#define RRC_TYPE_HELLO 1
#define RRC_TYPE_WELCOME 2
#define RRC_TYPE_JOIN 10
#define RRC_TYPE_JOINED 11
#define RRC_TYPE_PART 12
#define RRC_TYPE_PARTED 13
#define RRC_TYPE_MSG 20
#define RRC_TYPE_NOTICE 21
#define RRC_TYPE_ACTION 22
#define RRC_TYPE_PING 30
#define RRC_TYPE_PONG 31
#define RRC_TYPE_ERROR 40
#define RRC_TYPE_RESOURCE_ENVELOPE 50

#define RRC_EV_WELCOME 1
#define RRC_EV_JOINED 2
#define RRC_EV_PARTED 3
#define RRC_EV_MSG 4
#define RRC_EV_NOTICE 5
#define RRC_EV_ACTION 6
#define RRC_EV_ERROR 7
#define RRC_EV_PONG 8
#define RRC_EV_HELLO 9
#define RRC_EV_JOIN 10
#define RRC_EV_PART 11
#define RRC_EV_CLOSE 12
#define RRC_EV_TIMEOUT 13

typedef struct rrc_event {
	int kind;
	uint8_t sender[RRC_HASH_LEN];
	size_t sender_len;
	uint8_t peer[RRC_HASH_LEN];
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

/* config_path may point to a Reticulum-Go config file; every enabled
 * [interfaces] entry is attached (UDP, TCP client/server, Auto, Backbone
 * clients, and the rest the interface factory supports). Empty path uses
 * in-memory defaults with no interfaces. */
uint64_t rrc_node_create(const char *config_path);
int rrc_node_start(uint64_t node);
int rrc_node_stop(uint64_t node);
int rrc_node_destroy(uint64_t node);
int rrc_node_set_identity(uint64_t node, uint64_t identity);
int rrc_node_add_udp_interface(uint64_t node, const char *name, const char *local_addr, const char *peer_addr);
/* target/listen take "host:port". */
int rrc_node_add_tcp_client_interface(uint64_t node, const char *name, const char *target);
int rrc_node_add_tcp_server_interface(uint64_t node, const char *name, const char *listen);
int rrc_node_add_auto_interface(uint64_t node, const char *name);
int rrc_node_add_backbone_client_interface(uint64_t node, const char *name, const char *target);
int rrc_node_interface_count(uint64_t node);
int rrc_node_interface_names(uint64_t node, char *out, size_t out_len, size_t *written);
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

#ifdef __cplusplus
}
#endif

#endif
