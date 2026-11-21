#include <stdio.h>
#include <stdlib.h>
#include <string.h>

#include "rrc.h"

int main(void) {
	const char *ver = rrc_version();
	if (ver == NULL || strcmp(ver, RRC_API_VERSION) != 0) {
		fprintf(stderr, "unexpected version: %s\n", ver ? ver : "(null)");
		return 1;
	}

	uint64_t node = rrc_node_create("");
	if (node == 0) {
		fprintf(stderr, "rrc_node_create failed\n");
		return 1;
	}
	if (rrc_node_start(node) != RRC_OK) {
		fprintf(stderr, "rrc_node_start failed\n");
		rrc_node_destroy(node);
		return 1;
	}
	if (rrc_node_stop(node) != RRC_OK || rrc_node_destroy(node) != RRC_OK) {
		fprintf(stderr, "teardown failed\n");
		return 1;
	}

	printf("c-rrc-smoke ok\n");
	return 0;
}
