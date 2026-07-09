// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package rnsutil

import (
	"encoding/hex"
	"fmt"
	"net"
	"strconv"
	"time"

	"quad4/msgpack/v5/pkg/msgpack"
	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/reticulumconfig"
	"quad4/reticulum-go/pkg/sharedinstance"
	"quad4/reticulum-go/pkg/transport"
)

// RPCClient is a one-shot shared-instance RPC client compatible with Python
// multiprocessing.connection + msgpack.
type RPCClient struct {
	addr    string
	network string
	authkey []byte
	timeout time.Duration
}

// DialRPC connects using cfg ports and authkey. Authkey may be nil to resolve
// from cfg.RPCKey or transport_identity.
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
	port := cfg.InstanceControlPort
	if port == 0 {
		port = reticulumconfig.DefaultInstanceControlPort
	}
	c := &RPCClient{
		authkey: append([]byte(nil), authkey...),
		timeout: 10 * time.Second,
		network: "tcp",
		addr:    net.JoinHostPort("127.0.0.1", strconv.Itoa(port)),
	}
	if cfg.SharedInstanceType == common.SharedInstanceUnix {
		name := cfg.InstanceName
		if name == "" {
			name = "default"
		}
		c.network = "unix"
		c.addr = "@" + "rns/" + name + "/rpc"
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

// GetLinkCount returns the active link count.
func (c *RPCClient) GetLinkCount() (int, error) {
	var out int
	err := c.Call(map[string]any{"get": "link_count"}, &out)
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

// HexHash encodes b as lowercase hex with no separators.
func HexHash(b []byte) string {
	return hex.EncodeToString(b)
}
