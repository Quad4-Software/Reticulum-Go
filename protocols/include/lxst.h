#ifndef LXST_H
#define LXST_H

#include <stddef.h>
#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

#define LXST_API_VERSION "1.0"
#define LXST_HASH_LEN 16

#define LXST_OK 0
#define LXST_ERR_INVALID_ARG 1
#define LXST_ERR_INVALID_HANDLE 2
#define LXST_ERR_INTERNAL 6
#define LXST_ERR_TRUNCATED 8

#define LXST_STATUS_BUSY 0x00
#define LXST_STATUS_REJECTED 0x01
#define LXST_STATUS_CALLING 0x02
#define LXST_STATUS_AVAILABLE 0x03
#define LXST_STATUS_RINGING 0x04
#define LXST_STATUS_CONNECTING 0x05
#define LXST_STATUS_ESTABLISHED 0x06

#define LXST_PREFERRED_MODE 0xF0
#define LXST_PREFERRED_PROFILE 0xFF

#define LXST_CODEC_RAW 0x00
#define LXST_CODEC_OPUS 0x01
#define LXST_CODEC_CODEC2 0x02
#define LXST_CODEC_NULL 0xFF

#define LXST_MODE_FULL_DUPLEX 0x01
#define LXST_MODE_HALF_DUPLEX 0x02

#define LXST_PROFILE_BANDWIDTH_ULTRA_LOW 0x10
#define LXST_PROFILE_BANDWIDTH_VERY_LOW 0x20
#define LXST_PROFILE_BANDWIDTH_LOW 0x30
#define LXST_PROFILE_QUALITY_MEDIUM 0x40
#define LXST_PROFILE_QUALITY_HIGH 0x50
#define LXST_PROFILE_QUALITY_MAX 0x60
#define LXST_PROFILE_LATENCY_ULTRA_LOW 0x70
#define LXST_PROFILE_LATENCY_LOW 0x80

#define LXST_MAX_SIGNALS 32
#define LXST_MAX_FRAMES 8
#define LXST_MAX_FRAME_BYTES 2048

const char *lxst_version(void);
int lxst_last_error(char *buf, size_t buf_len, size_t *written);

int lxst_pack_signalling(const int *signals, size_t signal_count,
	uint8_t *out, size_t out_len, size_t *written);

int lxst_pack_frame(uint8_t codec, const uint8_t *payload, size_t payload_len,
	uint8_t *out, size_t out_len, size_t *written);

uint64_t lxst_unpack(const uint8_t *data, size_t data_len);

int lxst_packet_signal_count(uint64_t packet, size_t *count);
int lxst_packet_signal_at(uint64_t packet, size_t index, int *signal);
int lxst_packet_frame_count(uint64_t packet, size_t *count);
int lxst_packet_frame_at(uint64_t packet, size_t index,
	uint8_t *out, size_t out_len, size_t *written);
int lxst_packet_destroy(uint64_t packet);

int lxst_split_frame(const uint8_t *frame, size_t frame_len,
	uint8_t *codec_out,
	uint8_t *payload_out, size_t payload_out_len, size_t *payload_written);

int lxst_telephony_hash(const uint8_t *identity_hash, size_t identity_len,
	uint8_t *out, size_t out_len, size_t *written);

int lxst_dest_hash(const uint8_t *identity_hash, size_t identity_len,
	const char *app_name, const char *aspect,
	uint8_t *out, size_t out_len, size_t *written);

int lxst_signal_preferred_mode(int mode);
int lxst_signal_preferred_profile(int profile);
int lxst_profile_from_name(const char *name);

#ifdef __cplusplus
}
#endif

#endif
