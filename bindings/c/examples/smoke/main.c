#include <stdio.h>
#include <stdlib.h>
#include <string.h>

#include "rns.h"

int main(void) {
	const char *ver = rns_version();
	if (ver == NULL || strcmp(ver, RNS_API_VERSION) != 0) {
		fprintf(stderr, "unexpected version: %s\n", ver ? ver : "(null)");
		return 1;
	}

	uint64_t node = rns_node_create("");
	if (node == 0) {
		fprintf(stderr, "rns_node_create failed: %s\n", rns_last_error(NULL, 0, NULL) == RNS_OK ? "" : "error");
		char errbuf[256];
		size_t n = 0;
		rns_last_error(errbuf, sizeof errbuf, &n);
		fprintf(stderr, "last error: %.*s\n", (int)n, errbuf);
		return 1;
	}

	if (rns_node_start(node) != RNS_OK) {
		fprintf(stderr, "rns_node_start failed\n");
		rns_node_destroy(node);
		return 1;
	}

	rns_event ev;
	memset(&ev, 0, sizeof ev);
	if (rns_event_poll(node, &ev, 10) != RNS_ERR_TIMEOUT) {
		fprintf(stderr, "expected timeout poll on idle node\n");
		rns_node_stop(node);
		rns_node_destroy(node);
		return 1;
	}

	if (rns_node_stop(node) != RNS_OK || rns_node_destroy(node) != RNS_OK) {
		fprintf(stderr, "teardown failed\n");
		return 1;
	}

	printf("librns-smoke ok\n");
	return 0;
}
