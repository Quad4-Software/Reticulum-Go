// SPDX-License-Identifier: LicenseRef-Reticulum

#include <cstdint>
#include <cstdio>
#include <cstring>

#include "rrc.h"

int main() {
	const char *ver = rrc_version();
	if (ver == nullptr || std::strcmp(ver, RRC_API_VERSION) != 0) {
		std::fprintf(stderr, "version mismatch\n");
		return 1;
	}
	std::uint64_t node = rrc_node_create("");
	if (node == 0) {
		return 1;
	}
	if (rrc_node_start(node) != RRC_OK) {
		return 1;
	}
	if (rrc_node_stop(node) != RRC_OK || rrc_node_destroy(node) != RRC_OK) {
		return 1;
	}
	std::printf("cpp-rrc-smoke ok\n");
	return 0;
}
