#include <stdio.h>
#include <stdlib.h>
#include <string.h>

#include "lxmf.h"

int main(void) {
	uint64_t identity = lxmf_identity_generate();
	if (identity == 0) {
		return 1;
	}

	uint8_t hash[LXMF_HASH_LEN];
	size_t hash_len = 0;
	if (lxmf_identity_hash(identity, hash, sizeof hash, &hash_len) != LXMF_OK || hash_len != LXMF_HASH_LEN) {
		return 1;
	}

	uint64_t msg = lxmf_message_create(hash, hash_len, hash, hash_len, "hi", "hello lxmf");
	if (msg == 0) {
		return 1;
	}

	uint8_t packed[65536];
	size_t packed_len = 0;
	if (lxmf_message_pack(msg, identity, packed, sizeof packed, &packed_len) != LXMF_OK || packed_len == 0) {
		return 1;
	}
	lxmf_message_destroy(msg);

	uint64_t got = lxmf_message_unpack(packed, packed_len);
	if (got == 0) {
		return 1;
	}

	char content[4096];
	size_t content_len = 0;
	if (lxmf_message_get_content(got, content, sizeof content, &content_len) != LXMF_OK) {
		return 1;
	}
	lxmf_message_destroy(got);
	lxmf_identity_destroy(identity);

	if (strncmp(content, "hello lxmf", content_len) != 0) {
		return 1;
	}

	printf("c-lxmf-smoke ok\n");
	return 0;
}
