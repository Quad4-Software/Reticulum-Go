-- Reticulum (RNS) Wireshark / tshark Lua dissector
-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2024-2026 Quad4.io
--
-- Load with:
--   tshark -X lua_script:tools/wireshark/rns.lua -r capture.pcap -Y rns
-- Or copy into Wireshark plugins and enable.
--
-- Decodes UDP payloads that look like Reticulum headers. Matches pkg/packet DecodeFrame names.

local rns = Proto("rns", "Reticulum")

local f_flags = ProtoField.uint8("rns.flags", "Flags", base.HEX)
local f_hops = ProtoField.uint8("rns.hops", "Hops", base.DEC)
local f_header_type = ProtoField.uint8("rns.header_type", "Header type", base.DEC, {[0]="H1",[1]="H2"}, 0x40)
local f_context_flag = ProtoField.bool("rns.context_flag", "Context flag", 8, nil, 0x20)
local f_prop_type = ProtoField.uint8("rns.prop_type", "Propagation", base.DEC, {[0]="BROADCAST",[1]="TRANSPORT"}, 0x10)
local f_dest_type = ProtoField.uint8("rns.dest_type", "Destination type", base.DEC, {
  [0]="SINGLE",[1]="GROUP",[2]="PLAIN",[3]="LINK"
}, 0x0C)
local f_packet_type = ProtoField.uint8("rns.packet_type", "Packet type", base.DEC, {
  [0]="DATA",[1]="ANNOUNCE",[2]="LINKREQUEST",[3]="PROOF"
}, 0x03)
local f_transport_id = ProtoField.bytes("rns.transport_id", "Transport ID")
local f_dest_hash = ProtoField.bytes("rns.destination_hash", "Destination hash")
local f_context = ProtoField.uint8("rns.context", "Context", base.HEX)
local f_context_name = ProtoField.string("rns.context_name", "Context name")
local f_data = ProtoField.bytes("rns.data", "Data")

rns.fields = {
  f_flags, f_hops, f_header_type, f_context_flag, f_prop_type, f_dest_type, f_packet_type,
  f_transport_id, f_dest_hash, f_context, f_context_name, f_data
}

local context_names = {
  [0x00]="NONE",
  [0x01]="RESOURCE",
  [0x02]="RESOURCE_ADV",
  [0x03]="RESOURCE_REQ",
  [0x04]="RESOURCE_HMU",
  [0x05]="RESOURCE_PRF",
  [0x06]="RESOURCE_ICL",
  [0x07]="RESOURCE_RCL",
  [0x08]="CACHE_REQUEST",
  [0x09]="REQUEST",
  [0x0A]="RESPONSE",
  [0x0B]="PATH_RESPONSE",
  [0x0C]="COMMAND",
  [0x0D]="COMMAND_STATUS",
  [0x0E]="CHANNEL",
  [0xFA]="KEEPALIVE",
  [0xFB]="LINKIDENTIFY",
  [0xFC]="LINKCLOSE",
  [0xFD]="LINKPROOF",
  [0xFE]="LRRTT",
  [0xFF]="LRPROOF"
}

local function context_name(c)
  return context_names[c] or string.format("CTX_%02X", c)
end

function rns.dissector(buf, pinfo, tree)
  local len = buf:len()
  if len < 3 then
    return 0
  end

  pinfo.cols.protocol = "RNS"
  local subtree = tree:add(rns, buf(), "Reticulum")
  local flags = buf(0,1):uint()
  subtree:add(f_flags, buf(0,1))
  subtree:add(f_hops, buf(1,1))
  subtree:add(f_header_type, buf(0,1))
  subtree:add(f_context_flag, buf(0,1))
  subtree:add(f_prop_type, buf(0,1))
  subtree:add(f_dest_type, buf(0,1))
  subtree:add(f_packet_type, buf(0,1))

  local header_type = bit.band(bit.rshift(flags, 6), 0x01)
  local packet_type = bit.band(flags, 0x03)
  local off = 2
  if header_type == 1 then
    if len < off + 32 + 1 then
      return 0
    end
    subtree:add(f_transport_id, buf(off, 16))
    off = off + 16
    subtree:add(f_dest_hash, buf(off, 16))
    off = off + 16
  else
    if len < off + 16 + 1 then
      return 0
    end
    subtree:add(f_dest_hash, buf(off, 16))
    off = off + 16
  end

  local ctx = buf(off,1):uint()
  subtree:add(f_context, buf(off,1))
  subtree:add(f_context_name, context_name(ctx))
  off = off + 1
  if off < len then
    subtree:add(f_data, buf(off))
  end

  local type_names = {[0]="DATA",[1]="ANNOUNCE",[2]="LINKREQUEST",[3]="PROOF"}
  pinfo.cols.info = string.format("%s %s hops=%d", type_names[packet_type] or "?", context_name(ctx), buf(1,1):uint())
  return len
end

local function looks_like_rns(buf)
  if buf:len() < 19 then
    return false
  end
  local flags = buf(0,1):uint()
  local packet_type = bit.band(flags, 0x03)
  if packet_type > 3 then
    return false
  end
  local header_type = bit.band(bit.rshift(flags, 6), 0x01)
  local need = 2 + 16 + 1
  if header_type == 1 then
    need = 2 + 32 + 1
  end
  return buf:len() >= need
end

local function heur_udp(buf, pinfo, tree)
  if not looks_like_rns(buf) then
    return false
  end
  rns.dissector(buf, pinfo, tree)
  return true
end

rns:register_heuristic("udp", heur_udp)

-- Common mesh TCP and local UDP ports (also dissected when heuristic matches).
local udp_table = DissectorTable.get("udp.port")
udp_table:add(4242, rns)
udp_table:add(7822, rns)

local tcp_table = DissectorTable.get("tcp.port")
tcp_table:add(4242, rns)
tcp_table:add(7822, rns)
