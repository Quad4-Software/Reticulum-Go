#!/usr/bin/env luajit
-- SPDX-License-Identifier: LicenseRef-Reticulum

package.path = "../../?.lua;" .. package.path

local rrc = require("rrc")

if rrc.version() ~= rrc.API_VERSION then
	os.exit(1)
end

local node = rrc.node_create("")
rrc.node_start(node)
rrc.node_stop(node)
rrc.node_destroy(node)

print("lua-smoke ok")
