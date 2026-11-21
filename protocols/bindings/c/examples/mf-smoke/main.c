#include <stdio.h>
#include <stdlib.h>
#include <string.h>

#include "mf.h"

int main(void) {
	uint8_t sender[MF_HASH_LEN];
	for (int i = 0; i < MF_HASH_LEN; i++) {
		sender[i] = (uint8_t)i;
	}
	uint8_t packed[512];
	size_t packed_len = 0;
	if (mf_pack(sender, sizeof sender, "hello mf", packed, sizeof packed, &packed_len) != MF_OK) {
		return 1;
	}
	uint8_t got_sender[MF_HASH_LEN];
	size_t sender_len = 0;
	char text[128];
	size_t text_len = 0;
	if (mf_unpack(packed, packed_len, got_sender, sizeof got_sender, &sender_len, text, sizeof text, &text_len) != MF_OK) {
		return 1;
	}
	if (sender_len != MF_HASH_LEN || strncmp(text, "hello mf", text_len) != 0) {
		return 1;
	}
	printf("c-mf-smoke ok\n");
	return 0;
}
