#ifndef RNS_H
#define RNS_H

#include <stddef.h>
#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

#define RNS_API_VERSION "1.5"

#define RNS_HASH_LEN 16

#define RNS_OK 0
#define RNS_ERR_INVALID_ARG 1
#define RNS_ERR_INVALID_HANDLE 2
#define RNS_ERR_NOT_FOUND 3
#define RNS_ERR_STATE 4
#define RNS_ERR_IO 5
#define RNS_ERR_INTERNAL 6
#define RNS_ERR_TIMEOUT 7
#define RNS_ERR_TRUNCATED 8

#define RNS_EV_ANNOUNCE 1
#define RNS_EV_LINK_ESTABLISHED 2
#define RNS_EV_LINK_FAILED 3
#define RNS_EV_LINK_DATA 4
#define RNS_EV_LINK_CLOSED 5
#define RNS_EV_REQUEST_INCOMING 6
#define RNS_EV_REQUEST_RESPONSE 7
#define RNS_EV_REQUEST_FAILED 8
#define RNS_EV_RESOURCE_STARTED 9
#define RNS_EV_RESOURCE_CONCLUDED 10
#define RNS_EV_DESTINATION_DATA 11

typedef struct rns_event {
	int kind;
	uint8_t link_id[RNS_HASH_LEN];
	size_t link_id_len;
	uint8_t destination_hash[RNS_HASH_LEN];
	size_t destination_hash_len;
	uint8_t identity_hash[RNS_HASH_LEN];
	size_t identity_hash_len;
	uint8_t request_id[RNS_HASH_LEN];
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
	uint8_t hash[RNS_HASH_LEN];
	size_t hash_len;
	uint8_t via[RNS_HASH_LEN];
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

typedef void (*rns_event_callback)(const rns_event *event, void *user_data);

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
int rns_rsg_sign_file(uint64_t identity, const char *path,
	uint8_t *out, size_t out_len, size_t *written);
int rns_rsg_verify_file(const uint8_t *rsg, size_t rsg_len, const char *path,
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
int rns_destination_encrypt(const uint8_t *dest_hash, const uint8_t *plaintext, size_t plaintext_len,
	uint8_t *out, size_t out_len, size_t *written);
int rns_packet_send(uint64_t node, const uint8_t *dest_hash, const uint8_t *plaintext, size_t plaintext_len);

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
int rns_set_event_callback(uint64_t node, rns_event_callback callback, void *user_data);

#ifdef __cplusplus
}
#endif

#endif
