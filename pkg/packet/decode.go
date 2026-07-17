// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package packet

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
)

// Frame is a language-neutral decode of one Reticulum packet header and framing.
// Used by rgodump, vector replay, and Wireshark golden checks.
type Frame struct {
	OK              bool   `json:"ok"`
	Error           string `json:"error,omitempty"`
	RawHex          string `json:"raw_hex,omitempty"`
	RawLen          int    `json:"raw_len"`
	Flags           byte   `json:"flags"`
	Hops            byte   `json:"hops"`
	HeaderType      byte   `json:"header_type"`
	ContextFlag     bool   `json:"context_flag"`
	TransportType   byte   `json:"transport_type"`
	DestinationType byte   `json:"destination_type"`
	PacketType      byte   `json:"packet_type"`
	PacketTypeName  string `json:"packet_type_name"`
	DestTypeName    string `json:"destination_type_name"`
	Context         byte   `json:"context"`
	ContextName     string `json:"context_name"`
	DestinationHash string `json:"destination_hash,omitempty"`
	TransportID     string `json:"transport_id,omitempty"`
	DataLen         int    `json:"data_len"`
	DataHexPrefix   string `json:"data_hex_prefix,omitempty"`
}

// DecodeFrame unpacks raw wire bytes into a Frame summary.
func DecodeFrame(raw []byte) Frame {
	f := Frame{RawLen: len(raw)}
	if len(raw) > 0 {
		f.RawHex = hex.EncodeToString(raw)
	}
	if len(raw) < MinPacketSize {
		f.Error = "packet too short"
		return f
	}
	p := &Packet{Raw: append([]byte(nil), raw...)}
	if err := p.Unpack(); err != nil {
		f.Error = err.Error()
		f.Flags = raw[0]
		if len(raw) > 1 {
			f.Hops = raw[1]
		}
		return f
	}
	f.OK = true
	f.Flags = raw[0]
	f.Hops = p.Hops
	f.HeaderType = p.HeaderType
	f.ContextFlag = p.ContextFlag == FlagSet
	f.TransportType = p.TransportType
	f.DestinationType = p.DestinationType
	f.PacketType = p.PacketType
	f.PacketTypeName = PacketTypeName(p.PacketType)
	f.DestTypeName = DestinationTypeName(p.DestinationType)
	f.Context = p.Context
	f.ContextName = ContextName(p.Context)
	if len(p.DestinationHash) > 0 {
		f.DestinationHash = hex.EncodeToString(p.DestinationHash)
	}
	if len(p.TransportID) > 0 {
		f.TransportID = hex.EncodeToString(p.TransportID)
	}
	f.DataLen = len(p.Data)
	if len(p.Data) > 0 {
		n := min(len(p.Data), 32)
		f.DataHexPrefix = hex.EncodeToString(p.Data[:n])
	}
	return f
}

// PacketTypeName returns a stable wire name for packet type.
func PacketTypeName(t byte) string {
	switch t {
	case PacketTypeData:
		return "DATA"
	case PacketTypeAnnounce:
		return "ANNOUNCE"
	case PacketTypeLinkReq:
		return "LINKREQUEST"
	case PacketTypeProof:
		return "PROOF"
	default:
		return fmt.Sprintf("UNKNOWN_%02x", t)
	}
}

// DestinationTypeName returns a stable name for destination type.
func DestinationTypeName(t byte) string {
	switch t {
	case DestinationSingle:
		return "SINGLE"
	case DestinationGroup:
		return "GROUP"
	case DestinationPlain:
		return "PLAIN"
	case DestinationLink:
		return "LINK"
	default:
		return fmt.Sprintf("UNKNOWN_%02x", t)
	}
}

// ContextName returns a stable name for context byte.
func ContextName(c byte) string {
	switch c {
	case ContextNone:
		return "NONE"
	case ContextResource:
		return "RESOURCE"
	case ContextResourceAdv:
		return "RESOURCE_ADV"
	case ContextResourceReq:
		return "RESOURCE_REQ"
	case ContextResourceHMU:
		return "RESOURCE_HMU"
	case ContextResourcePRF:
		return "RESOURCE_PRF"
	case ContextResourceICL:
		return "RESOURCE_ICL"
	case ContextResourceRCL:
		return "RESOURCE_RCL"
	case ContextCacheReq:
		return "CACHE_REQUEST"
	case ContextRequest:
		return "REQUEST"
	case ContextResponse:
		return "RESPONSE"
	case ContextPathResponse:
		return "PATH_RESPONSE"
	case ContextCommand:
		return "COMMAND"
	case ContextCmdStatus:
		return "COMMAND_STATUS"
	case ContextChannel:
		return "CHANNEL"
	case ContextKeepalive:
		return "KEEPALIVE"
	case ContextLinkIdentify:
		return "LINKIDENTIFY"
	case ContextLinkClose:
		return "LINKCLOSE"
	case ContextLinkProof:
		return "LINKPROOF"
	case ContextLRRTT:
		return "LRRTT"
	case ContextLRProof:
		return "LRPROOF"
	default:
		return fmt.Sprintf("CTX_%02x", c)
	}
}

// MarshalJSONL writes one JSON object line for Frame.
func (f Frame) MarshalJSONL() ([]byte, error) {
	b, err := json.Marshal(f)
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}
