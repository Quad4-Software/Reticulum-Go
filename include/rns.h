#ifndef RNS_H
#define RNS_H

#include <stddef.h>
#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

#define RNS_API_VERSION "1.0"

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

typedef struct rns_event {
	int kind;
	uint8_t link_id[RNS_HASH_LEN];
	size_t link_id_len;
	uint8_t destination_hash[RNS_HASH_LEN];
	size_t destination_hash_len;
	uint8_t identity_hash[RNS_HASH_LEN];
	size_t identity_hash_len;
	uint8_t hops;
	char error_message[256];
	int error_message_truncated;
	uint8_t *app_data;
	size_t app_data_len;
	size_t app_data_cap;
	int app_data_truncated;
} rns_event;

const char *rns_version(void);

int rns_last_error(char *buf, size_t buf_len, size_t *written);

uint64_t rns_node_create(const char *config_path);
int rns_node_start(uint64_t node);
int rns_node_stop(uint64_t node);
int rns_node_destroy(uint64_t node);
int rns_node_set_identity(uint64_t node, uint64_t identity);

uint64_t rns_identity_generate(void);
uint64_t rns_identity_load(const char *path);
int rns_identity_destroy(uint64_t identity);
int rns_identity_hash(uint64_t identity, char *hex_buf, size_t hex_buf_len, size_t *written);

uint64_t rns_destination_create(uint64_t node, uint64_t identity, const char *app_name,
	const char *const *aspects, size_t aspect_count, int accepts_links);
int rns_destination_announce(uint64_t destination, const uint8_t *app_data, size_t app_data_len);
int rns_destination_hash(uint64_t destination, uint8_t *hash_out, size_t hash_out_len, size_t *written);
int rns_destination_destroy(uint64_t destination);

int rns_path_request(uint64_t node, const uint8_t *dest_hash);

uint64_t rns_link_open(uint64_t node, const uint8_t *dest_hash);
int rns_link_send(uint64_t link, const uint8_t *data, size_t data_len);
int rns_link_close(uint64_t link);
int rns_link_id(uint64_t link, uint8_t *id_out, size_t id_out_len, size_t *written);

int rns_event_poll(uint64_t node, rns_event *event, int timeout_ms);

#ifdef __cplusplus
}
#endif

#endif
