/*
 * SPDX-License-Identifier: Apache-2.0
 * Copyright (c) 2024-2026 Quad4.io
 *
 * NomadNet-style page fetch over librns.
 * Usage: librns-page-fetch [-c config] [-t timeout_sec] <dest_hash>:<page_path>
 */

#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <time.h>

#include "rns.h"

#define PAGE_BUF_CAP (512 * 1024)
#define PATH_TABLE_CAP 256
#define DEFAULT_TIMEOUT_SEC 60
#define PATH_RETRY_MS 2000

static void print_last_error(const char *what) {
	char errbuf[256];
	size_t n = 0;
	if (rns_last_error(errbuf, sizeof errbuf, &n) == RNS_OK && n > 0) {
		fprintf(stderr, "%s: %.*s\n", what, (int)n, errbuf);
	} else {
		fprintf(stderr, "%s\n", what);
	}
}

static int hex_nibble(char c) {
	if (c >= '0' && c <= '9') {
		return c - '0';
	}
	if (c >= 'a' && c <= 'f') {
		return c - 'a' + 10;
	}
	if (c >= 'A' && c <= 'F') {
		return c - 'A' + 10;
	}
	return -1;
}

static int hex_decode_hash(const char *hex, uint8_t out[RNS_HASH_LEN]) {
	size_t len = strlen(hex);
	if (len != RNS_HASH_LEN * 2) {
		return -1;
	}
	for (size_t i = 0; i < RNS_HASH_LEN; i++) {
		int hi = hex_nibble(hex[i * 2]);
		int lo = hex_nibble(hex[i * 2 + 1]);
		if (hi < 0 || lo < 0) {
			return -1;
		}
		out[i] = (uint8_t)((hi << 4) | lo);
	}
	return 0;
}

static int hash_eq(const uint8_t *a, size_t a_len, const uint8_t *b) {
	return a_len == RNS_HASH_LEN && memcmp(a, b, RNS_HASH_LEN) == 0;
}

static int path_known(uint64_t node, const uint8_t dest[RNS_HASH_LEN]) {
	rns_path_entry table[PATH_TABLE_CAP];
	size_t n = 0;
	if (rns_path_table(node, table, PATH_TABLE_CAP, &n, -1) != RNS_OK) {
		return 0;
	}
	for (size_t i = 0; i < n; i++) {
		if (hash_eq(table[i].hash, table[i].hash_len, dest)) {
			return 1;
		}
	}
	return 0;
}

static int64_t now_ms(void) {
	struct timespec ts;
	if (clock_gettime(CLOCK_MONOTONIC, &ts) != 0) {
		return 0;
	}
	return (int64_t)ts.tv_sec * 1000 + ts.tv_nsec / 1000000;
}

static void usage(const char *argv0) {
	fprintf(stderr,
		"Usage: %s [-c config] [-t timeout_sec] <dest_hash>:<page_path>\n"
		"\n"
		"Fetch a NomadNet / pageserver page over librns.\n"
		"\n"
		"Options:\n"
		"  -c path   Reticulum config file (required for network interfaces)\n"
		"  -t sec    Overall timeout in seconds (default %d)\n"
		"\n"
		"Example:\n"
		"  %s -c config.example 92798ea245a0afcfa559348e42d628c6:/page/index.mu\n",
		argv0, DEFAULT_TIMEOUT_SEC, argv0);
}

int main(int argc, char **argv) {
	const char *config_path = NULL;
	int timeout_sec = DEFAULT_TIMEOUT_SEC;
	const char *target = NULL;

	for (int i = 1; i < argc; i++) {
		if (strcmp(argv[i], "-c") == 0 && i + 1 < argc) {
			config_path = argv[++i];
			continue;
		}
		if (strcmp(argv[i], "-t") == 0 && i + 1 < argc) {
			timeout_sec = atoi(argv[++i]);
			if (timeout_sec <= 0) {
				fprintf(stderr, "timeout must be positive\n");
				return 1;
			}
			continue;
		}
		if (strcmp(argv[i], "-h") == 0 || strcmp(argv[i], "--help") == 0) {
			usage(argv[0]);
			return 0;
		}
		if (argv[i][0] == '-') {
			fprintf(stderr, "unknown option: %s\n", argv[i]);
			usage(argv[0]);
			return 1;
		}
		if (target != NULL) {
			fprintf(stderr, "extra argument: %s\n", argv[i]);
			usage(argv[0]);
			return 1;
		}
		target = argv[i];
	}

	if (target == NULL || config_path == NULL) {
		usage(argv[0]);
		return 1;
	}

	const char *colon = strchr(target, ':');
	if (colon == NULL || colon == target || colon[1] == '\0') {
		fprintf(stderr, "target must be <dest_hash>:<page_path>\n");
		return 1;
	}

	size_t hash_len = (size_t)(colon - target);
	if (hash_len != RNS_HASH_LEN * 2) {
		fprintf(stderr, "destination hash must be 32 hex characters\n");
		return 1;
	}

	char hash_hex[RNS_HASH_LEN * 2 + 1];
	memcpy(hash_hex, target, hash_len);
	hash_hex[hash_len] = '\0';
	const char *page_path = colon + 1;

	uint8_t dest_hash[RNS_HASH_LEN];
	if (hex_decode_hash(hash_hex, dest_hash) != 0) {
		fprintf(stderr, "invalid destination hash hex\n");
		return 1;
	}

	const char *ver = rns_version();
	if (ver == NULL || strcmp(ver, RNS_API_VERSION) != 0) {
		fprintf(stderr, "librns version mismatch: got %s want %s\n",
			ver ? ver : "(null)", RNS_API_VERSION);
		return 1;
	}

	uint64_t node = rns_node_create(config_path);
	if (node == 0) {
		print_last_error("rns_node_create failed");
		return 1;
	}

	uint64_t identity = rns_identity_generate();
	if (identity == 0) {
		print_last_error("rns_identity_generate failed");
		rns_node_destroy(node);
		return 1;
	}

	if (rns_node_set_identity(node, identity) != RNS_OK) {
		print_last_error("rns_node_set_identity failed");
		rns_identity_destroy(identity);
		rns_node_destroy(node);
		return 1;
	}

	if (rns_node_start(node) != RNS_OK) {
		print_last_error("rns_node_start failed");
		rns_identity_destroy(identity);
		rns_node_destroy(node);
		return 1;
	}

	printf("librns %s fetching %s from %s\n", ver, page_path, hash_hex);

	uint8_t *page_buf = malloc(PAGE_BUF_CAP);
	if (page_buf == NULL) {
		fprintf(stderr, "out of memory\n");
		rns_node_stop(node);
		rns_identity_destroy(identity);
		rns_node_destroy(node);
		return 1;
	}

	int64_t deadline = now_ms() + (int64_t)timeout_sec * 1000;
	int64_t last_path_req = 0;
	int saw_announce = 0;
	uint64_t link = 0;
	int rc = 1;

	while (now_ms() < deadline && link == 0) {
		int64_t now = now_ms();
		if (now - last_path_req >= PATH_RETRY_MS) {
			if (rns_path_request(node, dest_hash) != RNS_OK) {
				print_last_error("rns_path_request failed");
			}
			last_path_req = now;
			if (path_known(node, dest_hash)) {
				fprintf(stderr, "path known, waiting for destination identity announce\n");
			} else {
				fprintf(stderr, "requesting path to %s\n", hash_hex);
			}
		}

		rns_event ev;
		memset(&ev, 0, sizeof ev);
		ev.app_data = page_buf;
		ev.app_data_cap = PAGE_BUF_CAP;

		int poll = rns_event_poll(node, &ev, 200);
		if (poll == RNS_ERR_TIMEOUT) {
			if (saw_announce || path_known(node, dest_hash)) {
				link = rns_link_open(node, dest_hash);
				if (link == 0) {
					/* Identity may not be recalled yet. Keep waiting. */
				}
			}
			continue;
		}
		if (poll != RNS_OK) {
			print_last_error("rns_event_poll failed");
			break;
		}

		if (ev.kind == RNS_EV_ANNOUNCE &&
			hash_eq(ev.destination_hash, ev.destination_hash_len, dest_hash)) {
			saw_announce = 1;
			fprintf(stderr, "announce for target (hops=%u)\n", (unsigned)ev.hops);
			link = rns_link_open(node, dest_hash);
			if (link == 0) {
				print_last_error("rns_link_open after announce");
			}
		} else if (ev.kind == RNS_EV_LINK_FAILED) {
			fprintf(stderr, "link failed while opening: %s\n", ev.error_message);
		}
	}

	if (link == 0) {
		fprintf(stderr, "timed out before link open\n");
		goto cleanup;
	}

	int established = 0;
	while (now_ms() < deadline && !established) {
		rns_event ev;
		memset(&ev, 0, sizeof ev);
		ev.app_data = page_buf;
		ev.app_data_cap = PAGE_BUF_CAP;

		int poll = rns_event_poll(node, &ev, 500);
		if (poll == RNS_ERR_TIMEOUT) {
			continue;
		}
		if (poll != RNS_OK) {
			print_last_error("rns_event_poll failed");
			goto cleanup;
		}
		if (ev.kind == RNS_EV_LINK_ESTABLISHED) {
			established = 1;
			fprintf(stderr, "link established\n");
		} else if (ev.kind == RNS_EV_LINK_FAILED) {
			fprintf(stderr, "link establishment failed: %s\n", ev.error_message);
			goto cleanup;
		} else if (ev.kind == RNS_EV_LINK_CLOSED) {
			fprintf(stderr, "link closed before establish\n");
			goto cleanup;
		}
	}

	if (!established) {
		fprintf(stderr, "timed out waiting for link establishment\n");
		goto cleanup;
	}

	uint8_t request_id[RNS_HASH_LEN];
	size_t request_id_len = 0;
	int remaining_ms = (int)(deadline - now_ms());
	if (remaining_ms < 1000) {
		remaining_ms = 1000;
	}

	if (rns_link_request(node, link, page_path, NULL, 0, remaining_ms,
			request_id, sizeof request_id, &request_id_len) != RNS_OK) {
		print_last_error("rns_link_request failed");
		goto cleanup;
	}
	fprintf(stderr, "request sent for %s\n", page_path);

	while (now_ms() < deadline) {
		rns_event ev;
		memset(&ev, 0, sizeof ev);
		ev.app_data = page_buf;
		ev.app_data_cap = PAGE_BUF_CAP;

		int poll = rns_event_poll(node, &ev, 500);
		if (poll == RNS_ERR_TIMEOUT) {
			continue;
		}
		if (poll != RNS_OK) {
			print_last_error("rns_event_poll failed");
			goto cleanup;
		}

		if (ev.kind == RNS_EV_REQUEST_RESPONSE) {
			printf("\n=== Page Content (%zu bytes) ===\n", ev.app_data_len);
			if (ev.app_data_len > 0 && ev.app_data != NULL) {
				fwrite(ev.app_data, 1, ev.app_data_len, stdout);
				if (ev.app_data[ev.app_data_len - 1] != '\n') {
					putchar('\n');
				}
			}
			if (ev.app_data_truncated) {
				fprintf(stderr, "warning: response truncated to %d bytes\n", PAGE_BUF_CAP);
			}
			printf("=== End of Page ===\n");
			rc = 0;
			goto cleanup;
		}
		if (ev.kind == RNS_EV_REQUEST_FAILED) {
			fprintf(stderr, "request failed: %s\n", ev.error_message);
			goto cleanup;
		}
		if (ev.kind == RNS_EV_LINK_CLOSED) {
			fprintf(stderr, "link closed before response\n");
			goto cleanup;
		}
	}

	fprintf(stderr, "timed out waiting for page response\n");

cleanup:
	if (link != 0) {
		rns_link_close(link);
	}
	free(page_buf);
	rns_node_stop(node);
	rns_identity_destroy(identity);
	rns_node_destroy(node);
	return rc;
}
