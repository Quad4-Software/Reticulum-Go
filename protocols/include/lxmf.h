#ifndef LXMF_H
#define LXMF_H

#include <stddef.h>
#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

#define LXMF_API_VERSION "1.0"
#define LXMF_HASH_LEN 16

#define LXMF_OK 0
#define LXMF_ERR_INVALID_ARG 1
#define LXMF_ERR_INVALID_HANDLE 2
#define LXMF_ERR_INTERNAL 6
#define LXMF_ERR_TRUNCATED 8

const char *lxmf_version(void);
int lxmf_last_error(char *buf, size_t buf_len, size_t *written);

uint64_t lxmf_identity_generate(void);
uint64_t lxmf_identity_load(const char *path);
int lxmf_identity_save(uint64_t identity, const char *path);
int lxmf_identity_destroy(uint64_t identity);
int lxmf_identity_hash(uint64_t identity, uint8_t *out, size_t out_len, size_t *written);
int lxmf_identity_public_key(uint64_t identity, uint8_t *out, size_t out_len, size_t *written);
int lxmf_identity_delivery_hash(uint64_t identity, uint8_t *out, size_t out_len, size_t *written);
int lxmf_identity_register_recall(uint64_t identity);
int lxmf_identity_register_recall_source(const uint8_t *source, size_t source_len,
	const uint8_t *public_key, size_t public_key_len);

uint64_t lxmf_message_create(const uint8_t *dest, size_t dest_len,
	const uint8_t *source, size_t source_len,
	const char *title, const char *content);
int lxmf_message_pack(uint64_t message, uint64_t identity,
	uint8_t *out, size_t out_len, size_t *written);
uint64_t lxmf_message_unpack(const uint8_t *data, size_t data_len);
int lxmf_message_get_dest(uint64_t message, uint8_t *out, size_t out_len, size_t *written);
int lxmf_message_get_source(uint64_t message, uint8_t *out, size_t out_len, size_t *written);
int lxmf_message_get_title(uint64_t message, char *buf, size_t buf_len, size_t *written);
int lxmf_message_get_content(uint64_t message, char *buf, size_t buf_len, size_t *written);
int lxmf_message_set_fields_json(uint64_t message, const char *json);
int lxmf_message_fields_json(uint64_t message, char *buf, size_t buf_len, size_t *written);
int lxmf_message_field_count(uint64_t message, size_t *count);
uint64_t lxmf_message_unpack_verified(const uint8_t *data, size_t data_len, uint64_t identity);
int lxmf_message_encrypted_payload(uint64_t message, uint8_t *out, size_t out_len, size_t *written);
int lxmf_message_destroy(uint64_t message);

/* Pack for propagation upload. Message must be packable with identity.
 * recipient_hash: 16-byte LXMF delivery hash.
 * recipient_public_key: 64-byte Ed25519+X25519 public key material as used by Reticulum Identity (same as identity_public_key output).
 * pn_stamp_cost: prop-node stamp cost (0 = no PN stamp).
 * Writes msgpack outer payload suitable for link SendPacket / SendResource.
 */
int lxmf_message_pack_propagated(
	uint64_t message, uint64_t identity,
	const uint8_t *recipient_hash, size_t recipient_hash_len,
	const uint8_t *recipient_public_key, size_t recipient_public_key_len,
	int pn_stamp_cost,
	uint8_t *out, size_t out_len, size_t *written);

/* Stamp helpers (additive ABI 1.0).
 * lxmf_stamp_cost_from_app_data / lxmf_pn_stamp_cost_from_app_data return LXMF_OK with
 * *cost_out set, or LXMF_ERR_INVALID_ARG when the cost is absent (ok=false).
 * *cost_out may be 0 when absent.
 * lxmf_message_apply_stamp is a no-op returning LXMF_OK when stamp_cost <= 0.
 * timeout_ms <= 0 defaults to 60000.
 * lxmf_encode_announce_app_data builds v0.5 [name, cost?, icon?] announce app data.
 * Empty/NULL icon_name omits the icon element. fg_rgb3/bg_rgb3 may be NULL (treated as 0,0,0).
 */
int lxmf_stamp_cost_from_app_data(const uint8_t *app_data, size_t app_data_len, int64_t *cost_out);
int lxmf_pn_stamp_cost_from_app_data(const uint8_t *app_data, size_t app_data_len, int64_t *cost_out);
int lxmf_message_apply_stamp(uint64_t message, uint64_t identity, int stamp_cost, int timeout_ms);
int lxmf_encode_announce_app_data(const char *display_name, int64_t stamp_cost,
	const char *icon_name, const uint8_t *fg_rgb3, const uint8_t *bg_rgb3,
	uint8_t *out, size_t out_len, size_t *written);

#ifdef __cplusplus
}
#endif

#endif
