// SPDX-License-Identifier: LicenseRef-Reticulum

#include <cstdint>
#include <cstdio>
#include <cstring>

#include "mf.h"

int main() {
	std::uint8_t sender[MF_HASH_LEN];
	for (int i = 0; i < MF_HASH_LEN; i++) {
		sender[i] = static_cast<std::uint8_t>(i);
	}
	std::uint8_t packed[512];
	std::size_t packed_len = 0;
	if (mf_pack(sender, sizeof sender, "hello mf", packed, sizeof packed, &packed_len) != MF_OK) {
		return 1;
	}
	std::uint8_t got_sender[MF_HASH_LEN];
	std::size_t sender_len = 0;
	char text[128];
	std::size_t text_len = 0;
	if (mf_unpack(packed, packed_len, got_sender, sizeof got_sender, &sender_len, text, sizeof text, &text_len) != MF_OK) {
		return 1;
	}
	if (sender_len != MF_HASH_LEN || std::strncmp(text, "hello mf", text_len) != 0) {
		return 1;
	}
	std::printf("cpp-mf-smoke ok\n");
	return 0;
}
