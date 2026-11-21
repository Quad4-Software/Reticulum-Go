#define _DEFAULT_SOURCE
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>

#include "rrc.h"

#define HUB_LOCAL "127.0.0.1:42540"
#define HUB_PEER "127.0.0.1:42541"
#define CLI_LOCAL "127.0.0.1:42541"
#define CLI_PEER "127.0.0.1:42540"

static int wait_joined(uint64_t client) {
	for (int i = 0; i < 40; i++) {
		rrc_event ev;
		memset(&ev, 0, sizeof ev);
		int code = rrc_client_event_poll(client, 500, &ev);
		if (code == RRC_OK && ev.kind == RRC_EV_JOINED) {
			return 0;
		}
	}
	return 1;
}

static int wait_hub_msg(uint64_t hub, const char *want) {
	for (int i = 0; i < 40; i++) {
		rrc_event ev;
		memset(&ev, 0, sizeof ev);
		int code = rrc_hub_event_poll(hub, 500, &ev);
		if (code == RRC_OK && ev.kind == RRC_EV_MSG) {
			if (strncmp(ev.body, want, sizeof ev.body) == 0) {
				return 0;
			}
		}
	}
	return 1;
}

int main(void) {
	uint64_t hub_node = rrc_node_create("");
	uint64_t cli_node = rrc_node_create("");
	if (hub_node == 0 || cli_node == 0) {
		return 1;
	}

	rrc_node_add_udp_interface(hub_node, "H1", HUB_LOCAL, HUB_PEER);
	rrc_node_add_udp_interface(cli_node, "C1", CLI_LOCAL, CLI_PEER);

	uint64_t id_h = rrc_identity_generate();
	uint64_t id_c = rrc_identity_generate();
	rrc_node_set_identity(hub_node, id_h);
	rrc_node_set_identity(cli_node, id_c);
	rrc_node_start(hub_node);
	rrc_node_start(cli_node);

	uint64_t hub = rrc_hub_create(hub_node, id_h, "c-hub", "1.0");
	rrc_hub_start(hub);
	rrc_hub_announce(hub);

	uint8_t hub_hash[16];
	size_t n = 0;
	rrc_hub_hash(hub, hub_hash, sizeof hub_hash, &n);
	rrc_identity_seed_destination(id_h, hub_hash, n);

	int has = 0;
	for (int i = 0; i < 300; i++) {
		if (rrc_node_has_path(cli_node, hub_hash, n, &has) == RRC_OK && has) {
			break;
		}
		usleep(50000);
	}
	if (!has) {
		return 1;
	}

	uint64_t client = rrc_client_dial(cli_node, id_c, hub_hash, n, "alice", "c-client", "1.0", 15000);
	if (client == 0) {
		return 1;
	}
	if (rrc_client_join(client, "#lobby") != RRC_OK) {
		return 1;
	}
	if (wait_joined(client) != 0) {
		return 1;
	}
	const char *msg = "hello from c hub-client";
	if (rrc_client_send_msg(client, "#lobby", msg) != RRC_OK) {
		return 1;
	}
	if (wait_hub_msg(hub, msg) != 0) {
		return 1;
	}

	rrc_client_close(client);
	rrc_hub_destroy(hub);
	rrc_identity_destroy(id_h);
	rrc_identity_destroy(id_c);
	rrc_node_destroy(hub_node);
	rrc_node_destroy(cli_node);

	printf("c-hub-client ok\n");
	return 0;
}
