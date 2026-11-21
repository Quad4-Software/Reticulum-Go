#ifndef MF_H
#define MF_H

#include <stddef.h>
#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

#define MF_API_VERSION "1.0"
#define MF_HASH_LEN 16

#define MF_OK 0
#define MF_ERR_INVALID_ARG 1
#define MF_ERR_INTERNAL 6
#define MF_ERR_TRUNCATED 8

const char *mf_version(void);
int mf_last_error(char *buf, size_t buf_len, size_t *written);

int mf_pack(const uint8_t *sender, size_t sender_len, const char *text,
	uint8_t *out, size_t out_len, size_t *written);
int mf_unpack(const uint8_t *data, size_t data_len,
	uint8_t *sender_out, size_t sender_out_len, size_t *sender_written,
	char *text_out, size_t text_out_len, size_t *text_written);

#ifdef __cplusplus
}
#endif

#endif
