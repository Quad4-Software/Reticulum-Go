#include <stdio.h>
#include <stdlib.h>
#include <string.h>

#include "lxst.h"

int main(void) {
	int signals[] = {
		LXST_STATUS_AVAILABLE,
		lxst_signal_preferred_profile(LXST_PROFILE_QUALITY_MEDIUM),
		lxst_signal_preferred_mode(LXST_MODE_FULL_DUPLEX),
	};
	uint8_t packed[512];
	size_t packed_len = 0;
	if (lxst_pack_signalling(signals, sizeof signals / sizeof signals[0], packed, sizeof packed, &packed_len) != LXST_OK) {
		return 1;
	}
	uint64_t pkt = lxst_unpack(packed, packed_len);
	if (pkt == 0) {
		return 1;
	}
	size_t count = 0;
	if (lxst_packet_signal_count(pkt, &count) != LXST_OK || count != 3) {
		lxst_packet_destroy(pkt);
		return 1;
	}
	for (size_t i = 0; i < count; i++) {
		int sig = 0;
		if (lxst_packet_signal_at(pkt, i, &sig) != LXST_OK || sig != signals[i]) {
			lxst_packet_destroy(pkt);
			return 1;
		}
	}
	lxst_packet_destroy(pkt);

	uint8_t payload[] = {0xde, 0xad, 0xbe, 0xef};
	if (lxst_pack_frame(LXST_CODEC_OPUS, payload, sizeof payload, packed, sizeof packed, &packed_len) != LXST_OK) {
		return 1;
	}
	pkt = lxst_unpack(packed, packed_len);
	if (pkt == 0) {
		return 1;
	}
	uint8_t frame[512];
	size_t frame_len = 0;
	if (lxst_packet_frame_at(pkt, 0, frame, sizeof frame, &frame_len) != LXST_OK) {
		lxst_packet_destroy(pkt);
		return 1;
	}
	lxst_packet_destroy(pkt);

	uint8_t codec = 0;
	uint8_t got_payload[512];
	size_t got_len = 0;
	if (lxst_split_frame(frame, frame_len, &codec, got_payload, sizeof got_payload, &got_len) != LXST_OK) {
		return 1;
	}
	if (codec != LXST_CODEC_OPUS || got_len != sizeof payload || memcmp(got_payload, payload, got_len) != 0) {
		return 1;
	}

	printf("c-lxst-smoke ok\n");
	return 0;
}
