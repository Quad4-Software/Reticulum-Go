// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package rnsutil

import (
	"encoding/hex"
	"fmt"
	"maps"
	"net"
	"strconv"
	"strings"
	"time"

	"quad4/msgpack/v5/pkg/msgpack"
	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/reticulumconfig"
	"quad4/reticulum-go/pkg/sharedinstance"
	"quad4/reticulum-go/pkg/transport"
)

// RPCClient is a one-shot shared-instance RPC client over
// multiprocessing.connection framing and msgpack.
type RPCClient struct {
	addr       string
	network    string
	altAddr    string
	altNetwork string
	authkey    []byte
	timeout    time.Duration
}

func rpcDialTarget(cfg *common.ReticulumConfig, typ string) (network, addr string) {
	name := cfg.InstanceName
	if name == "" {
		name = "default"
	}
	if typ == common.SharedInstanceUnix {
		return "unix", "@" + "rns/" + name + "/rpc"
	}
	port := cfg.InstanceControlPort
	if port == 0 {
		port = reticulumconfig.DefaultInstanceControlPort
	}
	return "tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
}

// DialRPC connects using cfg ports and authkey. Authkey may be nil to resolve
// from cfg.RPCKey or transport_identity.
//
// When shared_instance_type is unset, the platform default is used (Unix on
// Linux, TCP elsewhere). On dial failure the other transport is tried so Go
// tools can reach stock Python rnsd without forcing TCP.
func DialRPC(cfg *common.ReticulumConfig, authkey []byte) (*RPCClient, error) {
	if cfg == nil {
		return nil, fmt.Errorf("nil config")
	}
	if len(authkey) == 0 {
		var err error
		authkey, err = ResolveAuthKey(cfg)
		if err != nil {
			return nil, err
		}
	}
	auto := strings.TrimSpace(cfg.SharedInstanceType) == ""
	typ := common.ResolveSharedInstanceType(cfg.SharedInstanceType)
	network, addr := rpcDialTarget(cfg, typ)
	c := &RPCClient{
		authkey: append([]byte(nil), authkey...),
		timeout: 10 * time.Second,
		network: network,
		addr:    addr,
	}
	if auto {
		altTyp := common.SharedInstanceTCP
		if typ == common.SharedInstanceTCP {
			altTyp = common.SharedInstanceUnix
		}
		c.altNetwork, c.altAddr = rpcDialTarget(cfg, altTyp)
	}
	return c, nil
}

// Addr returns the RPC dial target for diagnostics.
func (c *RPCClient) Addr() string {
	if c == nil {
		return ""
	}
	if c.network == "unix" {
		return c.network + ":" + c.addr
	}
	return c.addr
}

// SetTimeout sets the dial and I/O deadline budget.
func (c *RPCClient) SetTimeout(d time.Duration) {
	if c != nil && d > 0 {
		c.timeout = d
	}
}

// Call sends a msgpack request and unmarshals the response into dest.
func (c *RPCClient) Call(call map[string]any, dest any) error {
	if c == nil {
		return fmt.Errorf("nil rpc client")
	}
	conn, err := net.DialTimeout(c.network, c.addr, c.timeout)
	if err != nil && c.altNetwork != "" {
		alt, altErr := net.DialTimeout(c.altNetwork, c.altAddr, c.timeout)
		if altErr == nil {
			conn = alt
			err = nil
			c.network, c.addr = c.altNetwork, c.altAddr
			c.altNetwork, c.altAddr = "", ""
		}
	}
	if err != nil {
		return fmt.Errorf("dial shared instance rpc: %w", err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(c.timeout))

	if err := sharedinstance.AuthenticateClient(conn, c.authkey); err != nil {
		return fmt.Errorf("rpc auth: %w", err)
	}
	payload, err := msgpack.Marshal(call)
	if err != nil {
		return err
	}
	if err := sharedinstance.SendFramed(conn, payload); err != nil {
		return err
	}
	resp, err := sharedinstance.RecvFramed(conn, 1<<20)
	if err != nil {
		return err
	}
	if dest == nil {
		return nil
	}
	return msgpack.Unmarshal(resp, dest)
}

// GetInterfaceStats fetches interface statistics from a running instance.
func (c *RPCClient) GetInterfaceStats() (transport.InterfaceStatsResponse, error) {
	var out transport.InterfaceStatsResponse
	err := c.Call(map[string]any{"get": "interface_stats"}, &out)
	return out, err
}

// GetPathTable fetches the path table. maxHops may be nil for no filter.
func (c *RPCClient) GetPathTable(maxHops *int) ([]transport.PathTableEntry, error) {
	call := map[string]any{"get": "path_table"}
	if maxHops != nil {
		call["max_hops"] = *maxHops
	}
	var out []transport.PathTableEntry
	err := c.Call(call, &out)
	return out, err
}

// GetRateTable fetches announce rate-table rows.
func (c *RPCClient) GetRateTable() ([]transport.RateTableEntry, error) {
	var out []transport.RateTableEntry
	err := c.Call(map[string]any{"get": "rate_table"}, &out)
	return out, err
}

// GetLinkCount returns the link-table entry count.
func (c *RPCClient) GetLinkCount() (int, error) {
	var out int
	err := c.Call(map[string]any{"get": "link_count"}, &out)
	return out, err
}

// GetActiveLinkCount returns validated link-table entries.
func (c *RPCClient) GetActiveLinkCount() (int, error) {
	var out int
	err := c.Call(map[string]any{"get": "active_link_count"}, &out)
	return out, err
}

// GetNextHop returns the next-hop transport hash for destinationHash.
func (c *RPCClient) GetNextHop(destinationHash []byte) ([]byte, error) {
	var out []byte
	err := c.Call(map[string]any{
		"get":              "next_hop",
		"destination_hash": destinationHash,
	}, &out)
	return out, err
}

// GetNextHopIfName returns the egress interface name for destinationHash.
func (c *RPCClient) GetNextHopIfName(destinationHash []byte) (string, error) {
	var out string
	err := c.Call(map[string]any{
		"get":              "next_hop_if_name",
		"destination_hash": destinationHash,
	}, &out)
	return out, err
}

// GetFirstHopTimeout returns the first-hop timeout in seconds.
func (c *RPCClient) GetFirstHopTimeout(destinationHash []byte) (float64, error) {
	var out float64
	err := c.Call(map[string]any{
		"get":              "first_hop_timeout",
		"destination_hash": destinationHash,
	}, &out)
	return out, err
}

// DropPath removes a cached path. Returns whether the path existed.
func (c *RPCClient) DropPath(destinationHash []byte) (bool, error) {
	var out bool
	err := c.Call(map[string]any{
		"drop":             "path",
		"destination_hash": destinationHash,
	}, &out)
	return out, err
}

// DropAllVia removes all paths via transportHash. Returns drop count.
func (c *RPCClient) DropAllVia(transportHash []byte) (int, error) {
	var out int
	err := c.Call(map[string]any{
		"drop":             "all_via",
		"destination_hash": transportHash,
	}, &out)
	return out, err
}

// DropAnnounceQueues clears per-interface outgoing announce queues.
// Returns the number of queued announces dropped.
func (c *RPCClient) DropAnnounceQueues() (int, error) {
	var out int
	err := c.Call(map[string]any{"drop": "announce_queues"}, &out)
	return out, err
}

// GetBlackholedIdentities returns blackhole table rows from the instance.
// Accepts map or list shapes from the RPC payload and normalizes them.
func (c *RPCClient) GetBlackholedIdentities() ([]map[string]any, error) {
	var raw any
	if err := c.Call(map[string]any{"get": "blackholed_identities"}, &raw); err != nil {
		return nil, err
	}
	return flattenBlackholeRPC(raw), nil
}

func flattenBlackholeRPC(raw any) []map[string]any {
	switch v := raw.(type) {
	case []any:
		out := make([]map[string]any, 0, len(v))
		for _, item := range v {
			if m, ok := item.(map[string]any); ok {
				out = append(out, m)
			}
		}
		return out
	case []map[string]any:
		return v
	case map[string]any:
		out := make([]map[string]any, 0, len(v))
		for k, val := range v {
			m, ok := val.(map[string]any)
			if !ok {
				continue
			}
			row := make(map[string]any, len(m)+1)
			maps.Copy(row, m)
			if _, has := row["identity"]; !has {
				if b := []byte(k); len(b) == 16 {
					row["identity"] = b
				} else if h, err := hex.DecodeString(k); err == nil && len(h) == 16 {
					row["identity"] = h
				}
			}
			out = append(out, row)
		}
		return out
	default:
		return nil
	}
}

// BlackholeIdentity adds an identity to the blackhole table.
// until is a Unix timestamp (0 = indefinite). Returns true when inserted.
func (c *RPCClient) BlackholeIdentity(identityHash []byte, until float64, reason string) (bool, error) {
	call := map[string]any{"blackhole_identity": identityHash}
	if until > 0 {
		call["until"] = until
	}
	if reason != "" {
		call["reason"] = reason
	}
	var out bool
	err := c.Call(call, &out)
	return out, err
}

// UnblackholeIdentity removes an identity from the blackhole table.
func (c *RPCClient) UnblackholeIdentity(identityHash []byte) (bool, error) {
	var out bool
	err := c.Call(map[string]any{"unblackhole_identity": identityHash}, &out)
	return out, err
}

// HexHash encodes b as lowercase hex with no separators.
func HexHash(b []byte) string {
	return hex.EncodeToString(b)
}

// ParseDestHash decodes a 32-character hex truncated hash.
// Angle brackets from pretty-printed output are accepted.
func ParseDestHash(s string) ([]byte, error) {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "<")
	s = strings.TrimSuffix(s, ">")
	if len(s) != 32 {
		return nil, fmt.Errorf("hash length is invalid, must be 32 hexadecimal characters (16 bytes)")
	}
	b, err := hex.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("invalid hash: %w", err)
	}
	return b, nil
}
