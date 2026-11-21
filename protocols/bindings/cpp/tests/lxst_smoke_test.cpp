// SPDX-License-Identifier: LicenseRef-Reticulum

#include <cstdint>
#include <cstdio>
#include <cstring>

#include "lxst.h"

int main() {
	int signals[] = {
		LXST_STATUS_AVAILABLE,
		lxst_signal_preferred_profile(LXST_PROFILE_QUALITY_MEDIUM),
		lxst_signal_preferred_mode(LXST_MODE_FULL_DUPLEX),
	};
	std::uint8_t packed[512];
	std::size_t packed_len = 0;
	if (lxst_pack_signalling(signals, sizeof signals / sizeof signals[0], packed, sizeof packed, &packed_len) != LXST_OK) {
		return 1;
	}
	std::uint64_t pkt = lxst_unpack(packed, packed_len);
	if (pkt == 0) {
		return 1;
	}
	std::size_t count = 0;
	if (lxst_packet_signal_count(pkt, &count) != LXST_OK || count != 3) {
		lxst_packet_destroy(pkt);
		return 1;
	}
	for (std::size_t i = 0; i < count; i++) {
		int sig = 0;
		if (lxst_packet_signal_at(pkt, i, &sig) != LXST_OK || sig != signals[i]) {
			lxst_packet_destroy(pkt);
			return 1;
		}
	}
	lxst_packet_destroy(pkt);

	std::uint8_t identity[LXST_HASH_LEN];
	for (int i = 0; i < LXST_HASH_LEN; i++) {
		identity[i] = static_cast<std::uint8_t>(i + 1);
	}
	std::uint8_t hash[LXST_HASH_LEN];
	std::size_t hash_len = 0;
	if (lxst_telephony_hash(identity, sizeof identity, hash, sizeof hash, &hash_len) != LXST_OK || hash_len != LXST_HASH_LEN) {
		return 1;
	}

	std::printf("cpp-lxst-smoke ok\n");
	return 0;
}
