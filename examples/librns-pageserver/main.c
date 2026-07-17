/*
 * SPDX-License-Identifier: Apache-2.0
 * Copyright (c) 2024-2026 Quad4.io
 *
 * NomadNet-style pageserver over librns.
 * Usage: librns-pageserver -c config [-a announce_sec] [-p page_path]
 */

#include <signal.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <time.h>

#include "rns.h"

#define DEFAULT_ANNOUNCE_SEC 900
#define DEFAULT_PAGE_PATH "/page/index.mu"
#define DEFAULT_FILE_PATH "/file/test.txt"
#define DEFAULT_IDENTITY_PATH "identity"
#define DEFAULT_FILE_FILE "files/test.txt"
#define PAGE_BODY_CAP (256 * 1024)
#define FILE_BODY_CAP (256 * 1024)
#define REQ_DATA_CAP (64 * 1024)

static volatile sig_atomic_t g_stop = 0;

static void on_signal(int sig) {
	(void)sig;
	g_stop = 1;
}

static void print_last_error(const char *what) {
	char errbuf[256];
	size_t n = 0;
	if (rns_last_error(errbuf, sizeof errbuf, &n) == RNS_OK && n > 0) {
		fprintf(stderr, "%s: %.*s\n", what, (int)n, errbuf);
	} else {
		fprintf(stderr, "%s\n", what);
	}
}

static int64_t now_ms(void) {
	struct timespec ts;
	if (clock_gettime(CLOCK_MONOTONIC, &ts) != 0) {
		return 0;
	}
	return (int64_t)ts.tv_sec * 1000 + ts.tv_nsec / 1000000;
}

static int load_page_file(const char *path, uint8_t *out, size_t out_cap, size_t *out_len) {
	FILE *f = fopen(path, "rb");
	if (f == NULL) {
		return -1;
	}
	size_t n = fread(out, 1, out_cap, f);
	int err = ferror(f);
	fclose(f);
	if (err) {
		return -1;
	}
	*out_len = n;
	return 0;
}

static void hex_encode(const uint8_t *in, size_t in_len, char *out, size_t out_cap) {
	static const char *digits = "0123456789abcdef";
	size_t need = in_len * 2 + 1;
	if (out_cap < need) {
		if (out_cap > 0) {
			out[0] = '\0';
		}
		return;
	}
	for (size_t i = 0; i < in_len; i++) {
		out[i * 2] = digits[(in[i] >> 4) & 0xf];
		out[i * 2 + 1] = digits[in[i] & 0xf];
	}
	out[in_len * 2] = '\0';
}

static void usage(const char *argv0) {
	fprintf(stderr,
		"Usage: %s -c config [-i identity] [-a announce_sec] [-p page_file] [-f file] [-P request_path]\n"
		"\n"
		"Serve a NomadNet-compatible /page/ handler over librns.\n"
		"Destination: nomadnetwork.node\n"
		"Announce app_data name: librns-c-pageserver\n"
		"\n"
		"Options:\n"
		"  -c path   Reticulum config file (required)\n"
		"  -i path   Persistent identity file (default %s)\n"
		"            Loaded when present, otherwise generated and saved\n"
		"  -a sec    Announce interval seconds (default %d, 0 = once)\n"
		"  -p file   Micron page file to serve (default pages/index.mu)\n"
		"  -f file   Download file to serve at /file/test.txt (default %s)\n"
		"  -P path   Request path to register (default %s)\n",
		argv0, DEFAULT_IDENTITY_PATH, DEFAULT_ANNOUNCE_SEC, DEFAULT_FILE_FILE, DEFAULT_PAGE_PATH);
}

int main(int argc, char **argv) {
	const char *config_path = NULL;
	const char *identity_path = DEFAULT_IDENTITY_PATH;
	const char *page_file = "pages/index.mu";
	const char *file_file = DEFAULT_FILE_FILE;
	const char *request_path = DEFAULT_PAGE_PATH;
	const char *file_path = DEFAULT_FILE_PATH;
	int announce_sec = DEFAULT_ANNOUNCE_SEC;

	for (int i = 1; i < argc; i++) {
		if (strcmp(argv[i], "-c") == 0 && i + 1 < argc) {
			config_path = argv[++i];
			continue;
		}
		if (strcmp(argv[i], "-i") == 0 && i + 1 < argc) {
			identity_path = argv[++i];
			continue;
		}
		if (strcmp(argv[i], "-a") == 0 && i + 1 < argc) {
			announce_sec = atoi(argv[++i]);
			if (announce_sec < 0) {
				fprintf(stderr, "announce interval must be >= 0\n");
				return 1;
			}
			continue;
		}
		if (strcmp(argv[i], "-p") == 0 && i + 1 < argc) {
			page_file = argv[++i];
			continue;
		}
		if (strcmp(argv[i], "-f") == 0 && i + 1 < argc) {
			file_file = argv[++i];
			continue;
		}
		if (strcmp(argv[i], "-P") == 0 && i + 1 < argc) {
			request_path = argv[++i];
			continue;
		}
		if (strcmp(argv[i], "-h") == 0 || strcmp(argv[i], "--help") == 0) {
			usage(argv[0]);
			return 0;
		}
		fprintf(stderr, "unknown option: %s\n", argv[i]);
		usage(argv[0]);
		return 1;
	}

	if (config_path == NULL) {
		usage(argv[0]);
		return 1;
	}

	uint8_t *page_body = malloc(PAGE_BODY_CAP);
	uint8_t *file_body = malloc(FILE_BODY_CAP);
	uint8_t *req_buf = malloc(REQ_DATA_CAP);
	if (page_body == NULL || file_body == NULL || req_buf == NULL) {
		fprintf(stderr, "out of memory\n");
		free(page_body);
		free(file_body);
		free(req_buf);
		return 1;
	}

	size_t page_len = 0;
	if (load_page_file(page_file, page_body, PAGE_BODY_CAP, &page_len) != 0) {
		const char *fallback =
			"> C pageserver\n\n"
			"librns via Reticulum-Go\n\n"
			"Fallback page (file not found).\n\n"
			"`[Home`:/page/index.mu]\n"
			"`[Download Test File`:/file/test.txt]`_`f\n\n"
			"---\n";
		page_len = strlen(fallback);
		if (page_len > PAGE_BODY_CAP) {
			page_len = PAGE_BODY_CAP;
		}
		memcpy(page_body, fallback, page_len);
		fprintf(stderr, "warning: could not read %s, using built-in page\n", page_file);
	}

	size_t file_len = 0;
	if (load_page_file(file_file, file_body, FILE_BODY_CAP, &file_len) != 0) {
		const char *fallback_file = "Test file from Reticulum-Go node!\n";
		file_len = strlen(fallback_file);
		memcpy(file_body, fallback_file, file_len);
		fprintf(stderr, "warning: could not read %s, using built-in file\n", file_file);
	}

	const char *ver = rns_version();
	if (ver == NULL || strcmp(ver, RNS_API_VERSION) != 0) {
		fprintf(stderr, "librns version mismatch: got %s want %s\n",
			ver ? ver : "(null)", RNS_API_VERSION);
		free(page_body);
		free(file_body);
		free(req_buf);
		return 1;
	}

	uint64_t node = rns_node_create(config_path);
	if (node == 0) {
		print_last_error("rns_node_create failed");
		free(page_body);
		free(file_body);
		free(req_buf);
		return 1;
	}

	uint64_t identity = rns_identity_load(identity_path);
	if (identity == 0) {
		identity = rns_identity_generate();
		if (identity == 0) {
			print_last_error("rns_identity_generate failed");
			rns_node_destroy(node);
			free(page_body);
			free(file_body);
			free(req_buf);
			return 1;
		}
		if (rns_identity_save(identity, identity_path) != RNS_OK) {
			print_last_error("rns_identity_save failed");
			rns_identity_destroy(identity);
			rns_node_destroy(node);
			free(page_body);
			free(file_body);
			free(req_buf);
			return 1;
		}
		fprintf(stderr, "created and saved identity to %s\n", identity_path);
	} else {
		fprintf(stderr, "loaded identity from %s\n", identity_path);
	}

	if (rns_node_set_identity(node, identity) != RNS_OK) {
		print_last_error("rns_node_set_identity failed");
		rns_identity_destroy(identity);
		rns_node_destroy(node);
		free(page_body);
		free(file_body);
		free(req_buf);
		return 1;
	}

	if (rns_node_start(node) != RNS_OK) {
		print_last_error("rns_node_start failed");
		rns_identity_destroy(identity);
		rns_node_destroy(node);
		free(page_body);
		free(file_body);
		free(req_buf);
		return 1;
	}

	const char *aspects[] = {"node"};
	uint64_t dest = rns_destination_create(node, 0, "nomadnetwork", aspects, 1, 1);
	if (dest == 0) {
		print_last_error("rns_destination_create failed");
		rns_node_stop(node);
		rns_identity_destroy(identity);
		rns_node_destroy(node);
		free(page_body);
		free(file_body);
		free(req_buf);
		return 1;
	}

	if (rns_destination_register_request_handler(dest, request_path) != RNS_OK) {
		print_last_error("rns_destination_register_request_handler failed");
		rns_destination_destroy(dest);
		rns_node_stop(node);
		rns_identity_destroy(identity);
		rns_node_destroy(node);
		free(page_body);
		free(file_body);
		free(req_buf);
		return 1;
	}

	if (rns_destination_register_request_handler(dest, file_path) != RNS_OK) {
		print_last_error("rns_destination_register_request_handler file failed");
		rns_destination_destroy(dest);
		rns_node_stop(node);
		rns_identity_destroy(identity);
		rns_node_destroy(node);
		free(page_body);
		free(file_body);
		free(req_buf);
		return 1;
	}

	uint8_t dest_hash[RNS_HASH_LEN];
	size_t dest_hash_len = 0;
	if (rns_destination_hash(dest, dest_hash, sizeof dest_hash, &dest_hash_len) != RNS_OK ||
		dest_hash_len != RNS_HASH_LEN) {
		print_last_error("rns_destination_hash failed");
		rns_destination_destroy(dest);
		rns_node_stop(node);
		rns_identity_destroy(identity);
		rns_node_destroy(node);
		free(page_body);
		free(file_body);
		free(req_buf);
		return 1;
	}

	char dest_hex[RNS_HASH_LEN * 2 + 1];
	hex_encode(dest_hash, RNS_HASH_LEN, dest_hex, sizeof dest_hex);

	signal(SIGINT, on_signal);
	signal(SIGTERM, on_signal);

	printf("DEST_HASH=%s\n", dest_hex);
	printf("REQUEST_PATH=%s\n", request_path);
	printf("FILE_PATH=%s\n", file_path);
	fflush(stdout);
	fprintf(stderr, "librns %s pageserver listening as nomadnetwork.node\n", ver);
	fprintf(stderr, "announce name=librns-c-pageserver interval=%ds\n", announce_sec);
	fprintf(stderr, "serving %zu bytes from %s\n", page_len, page_file);
	fprintf(stderr, "serving %zu bytes from %s as %s\n", file_len, file_file, file_path);

	const char *app_data = "librns-c-pageserver";
	if (rns_destination_announce(dest, (const uint8_t *)app_data, strlen(app_data)) != RNS_OK) {
		print_last_error("rns_destination_announce failed");
	} else {
		fprintf(stderr, "announce sent\n");
	}

	int64_t last_announce = now_ms();
	int64_t announce_ms = (int64_t)announce_sec * 1000;

	while (!g_stop) {
		int64_t now = now_ms();
		if (announce_sec > 0 && now - last_announce >= announce_ms) {
			if (rns_destination_announce(dest, (const uint8_t *)app_data, strlen(app_data)) == RNS_OK) {
				fprintf(stderr, "announce refreshed\n");
			}
			last_announce = now;
		}

		rns_event ev;
		memset(&ev, 0, sizeof ev);
		ev.app_data = req_buf;
		ev.app_data_cap = REQ_DATA_CAP;

		int poll = rns_event_poll(node, &ev, 200);
		if (poll == RNS_ERR_TIMEOUT) {
			continue;
		}
		if (poll != RNS_OK) {
			print_last_error("rns_event_poll failed");
			break;
		}

		if (ev.kind == RNS_EV_LINK_ESTABLISHED) {
			fprintf(stderr, "inbound link established\n");
			continue;
		}
		if (ev.kind == RNS_EV_LINK_CLOSED) {
			fprintf(stderr, "link closed\n");
			continue;
		}
		if (ev.kind == RNS_EV_REQUEST_INCOMING) {
			fprintf(stderr, "request incoming path=%s\n", ev.path);
			if (strcmp(ev.path, request_path) == 0) {
				if (rns_request_respond(node, ev.request_id, ev.request_id_len,
						page_body, page_len) != RNS_OK) {
					print_last_error("rns_request_respond failed");
				} else {
					fprintf(stderr, "served %s (%zu bytes)\n", request_path, page_len);
				}
			} else if (strcmp(ev.path, file_path) == 0) {
				if (rns_request_respond(node, ev.request_id, ev.request_id_len,
						file_body, file_len) != RNS_OK) {
					print_last_error("rns_request_respond failed");
				} else {
					fprintf(stderr, "served %s (%zu bytes)\n", file_path, file_len);
				}
			} else {
				const char *msg = "page not found\n";
				if (rns_request_respond(node, ev.request_id, ev.request_id_len,
						(const uint8_t *)msg, strlen(msg)) != RNS_OK) {
					print_last_error("rns_request_respond failed");
				}
			}
		}
	}

	fprintf(stderr, "shutting down\n");
	rns_destination_destroy(dest);
	rns_node_stop(node);
	rns_identity_destroy(identity);
	rns_node_destroy(node);
	free(page_body);
	free(file_body);
	free(req_buf);
	return 0;
}
