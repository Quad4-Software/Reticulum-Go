// SPDX-License-Identifier: LicenseRef-Reticulum

#include <cstdint>
#include <cstdio>
#include <cstring>

#include "lxmf.h"

int main() {
	std::uint64_t identity = lxmf_identity_generate();
	if (identity == 0) {
		return 1;
	}

	std::uint8_t hash[LXMF_HASH_LEN];
	std::size_t hash_len = 0;
	if (lxmf_identity_hash(identity, hash, sizeof hash, &hash_len) != LXMF_OK || hash_len != LXMF_HASH_LEN) {
		return 1;
	}

	std::uint64_t msg = lxmf_message_create(hash, hash_len, hash, hash_len, "hi", "hello lxmf");
	if (msg == 0) {
		return 1;
	}

	std::uint8_t packed[65536];
	std::size_t packed_len = 0;
	if (lxmf_message_pack(msg, identity, packed, sizeof packed, &packed_len) != LXMF_OK || packed_len == 0) {
		return 1;
	}
	lxmf_message_destroy(msg);

	std::uint64_t got = lxmf_message_unpack(packed, packed_len);
	if (got == 0) {
		return 1;
	}

	char content[4096];
	std::size_t content_len = 0;
	if (lxmf_message_get_content(got, content, sizeof content, &content_len) != LXMF_OK) {
		return 1;
	}
	lxmf_message_destroy(got);
	lxmf_identity_destroy(identity);

	if (std::strncmp(content, "hello lxmf", content_len) != 0) {
		return 1;
	}

	std::printf("cpp-lxmf-smoke ok\n");
	return 0;
}
