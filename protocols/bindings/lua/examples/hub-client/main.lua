#!/usr/bin/env luajit
-- SPDX-License-Identifier: LicenseRef-Reticulum

package.path = "./?.lua;../../?.lua;../../rrc/?.lua;" .. package.path

local rrc = require("rrc")

local HUB_LOCAL = "127.0.0.1:42570"
local HUB_PEER = "127.0.0.1:42571"
local CLI_LOCAL = "127.0.0.1:42571"
local CLI_PEER = "127.0.0.1:42570"

local hub_node = rrc.node_create("")
local cli_node = rrc.node_create("")
rrc.node_add_udp_interface(hub_node, "H1", HUB_LOCAL, HUB_PEER)
rrc.node_add_udp_interface(cli_node, "C1", CLI_LOCAL, CLI_PEER)

local id_h = rrc.identity_generate()
local id_c = rrc.identity_generate()
rrc.node_set_identity(hub_node, id_h)
rrc.node_set_identity(cli_node, id_c)
rrc.node_start(hub_node)
rrc.node_start(cli_node)

local hub = rrc.hub_create(hub_node, id_h, "lua-hub", "1.0")
rrc.hub_start(hub)
rrc.hub_announce(hub)
local hub_hash = rrc.hub_hash(hub)
rrc.identity_seed_destination(id_h, hub_hash)

local deadline = os.clock() + 15
while os.clock() < deadline do
	if rrc.node_has_path(cli_node, hub_hash) then
		break
	end
	rrc.sleep_ms(50)
end
if not rrc.node_has_path(cli_node, hub_hash) then
	io.stderr:write("path timeout\n")
	os.exit(1)
end

local client = rrc.client_dial(cli_node, id_c, hub_hash, "alice", "lua-client", "1.0", 15000)
rrc.client_join(client, "#lobby")

local joined = false
local join_end = os.clock() + 10
while os.clock() < join_end do
	local ev, err = rrc.client_event_poll(client, 500)
	if ev and ev.kind == rrc.RRC_EV_JOINED then
		joined = true
		break
	end
	if err ~= "timeout" and err ~= nil then
		io.stderr:write(tostring(err) .. "\n")
		os.exit(1)
	end
end
if not joined then
	io.stderr:write("join timeout\n")
	os.exit(1)
end

local want = "hello from lua hub-client"
rrc.client_send_msg(client, "#lobby", want)

local msg_end = os.clock() + 10
while os.clock() < msg_end do
	local ev, err = rrc.hub_event_poll(hub, 500)
	if ev and ev.kind == rrc.RRC_EV_MSG and ev.body == want then
		print("lua-hub-client ok")
		rrc.client_close(client)
		rrc.hub_destroy(hub)
		rrc.identity_destroy(id_h)
		rrc.identity_destroy(id_c)
		rrc.node_destroy(hub_node)
		rrc.node_destroy(cli_node)
		os.exit(0)
	end
	if err ~= "timeout" and err ~= nil then
		io.stderr:write(tostring(err) .. "\n")
		os.exit(1)
	end
end

io.stderr:write("hub did not receive message\n")
os.exit(1)
