// SPDX-License-Identifier: LicenseRef-Reticulum

#include <cstdint>
#include <cstdio>
#include <cstdlib>
#include <cstring>
#include <fstream>
#include <string>

#include "lxmf.h"

static bool has_field_key(const std::string &json, const char *key) {
	const std::string needle = std::string("\"") + key + "\"";
	return json.find(needle) != std::string::npos;
}

static std::string read_file(const char *path) {
	std::ifstream in(path, std::ios::binary);
	if (!in) {
		return {};
	}
	return std::string((std::istreambuf_iterator<char>(in)), std::istreambuf_iterator<char>());
}

int main(int argc, char **argv) {
	const char *fields_path = "../../pkg/lxmf/testdata/messaging_interop_fields.json";
	if (argc > 1) {
		fields_path = argv[1];
	}

	const std::string fields_json = read_file(fields_path);
	if (fields_json.empty()) {
		std::fprintf(stderr, "failed to read fields json: %s\n", fields_path);
		return 1;
	}

	std::uint64_t identity = lxmf_identity_generate();
	if (identity == 0) {
		return 1;
	}

	std::uint8_t dest[LXMF_HASH_LEN];
	std::uint8_t source[LXMF_HASH_LEN];
	std::size_t dest_len = 0;
	std::size_t source_len = 0;

	if (lxmf_identity_delivery_hash(identity, dest, sizeof dest, &dest_len) != LXMF_OK ||
	    dest_len != LXMF_HASH_LEN) {
		return 1;
	}
	std::memcpy(source, dest, LXMF_HASH_LEN);
	source_len = dest_len;
	if (lxmf_identity_register_recall(identity) != LXMF_OK) {
		return 1;
	}

	std::uint64_t msg = lxmf_message_create(dest, dest_len, source, source_len, "interop", "messaging fields");
	if (msg == 0) {
		return 1;
	}
	if (lxmf_message_set_fields_json(msg, fields_json.c_str()) != LXMF_OK) {
		lxmf_message_destroy(msg);
		return 1;
	}

	std::uint8_t packed[65536];
	std::size_t packed_len = 0;
	if (lxmf_message_pack(msg, identity, packed, sizeof packed, &packed_len) != LXMF_OK || packed_len == 0) {
		lxmf_message_destroy(msg);
		return 1;
	}
	lxmf_message_destroy(msg);

	std::uint64_t got = lxmf_message_unpack_verified(packed, packed_len, identity);
	if (got == 0) {
		return 1;
	}

	char content[4096];
	std::size_t content_len = 0;
	if (lxmf_message_get_content(got, content, sizeof content, &content_len) != LXMF_OK) {
		lxmf_message_destroy(got);
		return 1;
	}
	if (std::strncmp(content, "messaging fields", content_len) != 0) {
		lxmf_message_destroy(got);
		return 1;
	}

	std::size_t field_count = 0;
	if (lxmf_message_field_count(got, &field_count) != LXMF_OK || field_count < 20) {
		lxmf_message_destroy(got);
		return 1;
	}

	char fields_out[262144];
	std::size_t fields_out_len = 0;
	if (lxmf_message_fields_json(got, fields_out, sizeof fields_out, &fields_out_len) != LXMF_OK) {
		lxmf_message_destroy(got);
		return 1;
	}
	fields_out[fields_out_len] = '\0';
	const std::string fields_str(fields_out, fields_out_len);
	lxmf_message_destroy(got);
	lxmf_identity_destroy(identity);

	static const char *keys[] = {
		"0x04", "0x05", "0x06", "0x07", "0x08", "0x09", "0x0a", "0x0b",
		"0x0c", "0x0d", "0x0e", "0x0f", "0x30", "0x31", "0x40", "0x41",
		"0x42", "0x02", "0x03", "0xfb", "0xfc", "0xfd", "0xfe", "0xff",
	};
	for (const char *key : keys) {
		if (!has_field_key(fields_str, key)) {
			std::fprintf(stderr, "missing field %s\n", key);
			return 1;
		}
	}

	std::printf("cpp-lxmf-interop ok fields=%zu\n", field_count);
	return 0;
}
