#include <stdio.h>
#include <stdlib.h>
#include <string.h>

#include "lxmf.h"

static char *read_file(const char *path, size_t *out_len) {
	FILE *f = fopen(path, "rb");
	if (!f) {
		return NULL;
	}
	if (fseek(f, 0, SEEK_END) != 0) {
		fclose(f);
		return NULL;
	}
	long size = ftell(f);
	if (size < 0) {
		fclose(f);
		return NULL;
	}
	if (fseek(f, 0, SEEK_SET) != 0) {
		fclose(f);
		return NULL;
	}
	char *buf = malloc((size_t)size + 1);
	if (!buf) {
		fclose(f);
		return NULL;
	}
	if (fread(buf, 1, (size_t)size, f) != (size_t)size) {
		free(buf);
		fclose(f);
		return NULL;
	}
	buf[size] = '\0';
	fclose(f);
	if (out_len) {
		*out_len = (size_t)size;
	}
	return buf;
}

static int has_field_key(const char *json, const char *key) {
	char needle[32];
	snprintf(needle, sizeof needle, "\"%s\"", key);
	return strstr(json, needle) != NULL;
}

int main(int argc, char **argv) {
	const char *fields_path = "../../../../pkg/lxmf/testdata/messaging_interop_fields.json";
	if (argc > 1) {
		fields_path = argv[1];
	}

	size_t fields_len = 0;
	char *fields_json = read_file(fields_path, &fields_len);
	if (!fields_json || fields_len == 0) {
		fprintf(stderr, "failed to read fields json: %s\n", fields_path);
		free(fields_json);
		return 1;
	}

	uint64_t identity = lxmf_identity_generate();
	if (identity == 0) {
		free(fields_json);
		return 1;
	}

	uint8_t dest[LXMF_HASH_LEN];
	uint8_t source[LXMF_HASH_LEN];
	size_t dest_len = 0;
	size_t source_len = 0;
	uint8_t public_key[128];
	size_t public_key_len = 0;

	if (lxmf_identity_delivery_hash(identity, dest, sizeof dest, &dest_len) != LXMF_OK ||
	    dest_len != LXMF_HASH_LEN) {
		free(fields_json);
		return 1;
	}
	memcpy(source, dest, LXMF_HASH_LEN);
	source_len = dest_len;
	if (lxmf_identity_public_key(identity, public_key, sizeof public_key, &public_key_len) != LXMF_OK ||
	    public_key_len == 0) {
		free(fields_json);
		return 1;
	}
	if (lxmf_identity_register_recall(identity) != LXMF_OK) {
		free(fields_json);
		return 1;
	}

	uint64_t msg = lxmf_message_create(dest, dest_len, source, source_len, "interop", "messaging fields");
	if (msg == 0) {
		free(fields_json);
		return 1;
	}
	if (lxmf_message_set_fields_json(msg, fields_json) != LXMF_OK) {
		lxmf_message_destroy(msg);
		free(fields_json);
		return 1;
	}
	free(fields_json);

	uint8_t packed[65536];
	size_t packed_len = 0;
	if (lxmf_message_pack(msg, identity, packed, sizeof packed, &packed_len) != LXMF_OK || packed_len == 0) {
		lxmf_message_destroy(msg);
		return 1;
	}
	lxmf_message_destroy(msg);

	uint64_t got = lxmf_message_unpack_verified(packed, packed_len, identity);
	if (got == 0) {
		return 1;
	}

	char content[4096];
	size_t content_len = 0;
	if (lxmf_message_get_content(got, content, sizeof content, &content_len) != LXMF_OK) {
		lxmf_message_destroy(got);
		return 1;
	}
	if (strncmp(content, "messaging fields", content_len) != 0) {
		lxmf_message_destroy(got);
		return 1;
	}

	size_t field_count = 0;
	if (lxmf_message_field_count(got, &field_count) != LXMF_OK || field_count < 20) {
		lxmf_message_destroy(got);
		return 1;
	}

	char fields_out[262144];
	size_t fields_out_len = 0;
	if (lxmf_message_fields_json(got, fields_out, sizeof fields_out, &fields_out_len) != LXMF_OK) {
		lxmf_message_destroy(got);
		return 1;
	}
	fields_out[fields_out_len] = '\0';
	lxmf_message_destroy(got);
	lxmf_identity_destroy(identity);

	static const char *keys[] = {
		"0x04", "0x05", "0x06", "0x07", "0x08", "0x09", "0x0a", "0x0b",
		"0x0c", "0x0d", "0x0e", "0x0f", "0x30", "0x31", "0x40", "0x41",
		"0x42", "0x02", "0x03", "0xfb", "0xfc", "0xfd", "0xfe", "0xff",
	};
	for (size_t i = 0; i < sizeof keys / sizeof keys[0]; i++) {
		if (!has_field_key(fields_out, keys[i])) {
			fprintf(stderr, "missing field %s\n", keys[i]);
			return 1;
		}
	}

	printf("c-lxmf-interop ok fields=%zu\n", field_count);
	return 0;
}
