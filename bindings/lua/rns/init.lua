-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2024-2026 Quad4.io

local errors = require("rns.errors")
local util = require("rns.util")
local Identity = require("rns.identity")
local Node = require("rns.node")
local Destination = require("rns.destination")
local link_mod = require("rns.link")
local path_mod = require("rns.path")
local event_mod = require("rns.event")
local interfaces_mod = require("rns.interfaces")
local rsg_mod = require("rns.rsg")

local M = {
	API_VERSION = "1.5",
	HASH_LEN = 16,
	Error = errors.Error,
	version = errors.version,
	last_error = errors.last_error,
	map_code = errors.map_code,
	hash_to_hex = util.hash_to_hex,
	hex_to_hash = util.hex_to_hash,
	Identity = Identity,
	Node = Node,
	Destination = Destination,
	Link = link_mod.Link,
	request_respond = link_mod.request_respond,
	request_respond_file = link_mod.request_respond_file,
	path_request = path_mod.path_request,
	path_table = path_mod.path_table,
	path_known = path_mod.path_known,
	Event = event_mod.Event,
	EventKind = event_mod.EventKind,
	hash_eq = event_mod.hash_eq,
	interfaces_list = interfaces_mod.interfaces_list,
	rsg_create = rsg_mod.rsg_create,
	rsg_validate = rsg_mod.rsg_validate,
	rsm_verify = rsg_mod.rsm_verify,
}

return M
