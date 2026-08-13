// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package rnsutil

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"quad4/msgpack/v5/pkg/msgpack"
	"quad4/reticulum-go/pkg/destination"
	"quad4/reticulum-go/pkg/identity"
	"quad4/reticulum-go/pkg/link"
	"quad4/reticulum-go/pkg/transport"
)

const (
	// RemoteManagementName is the dotted destination name Python uses for
	// hash_from_name_and_identity of the remote management dest.
	RemoteManagementName = "rnstransport.remote.management"
)

// RemoteManagementDestHash is the destination hash of
// rnstransport.remote.management for a transport identity hash.
func RemoteManagementDestHash(transportIdentityHash []byte) []byte {
	return destination.HashFromNameAndIdentity(RemoteManagementName, transportIdentityHash)
}

// ExpandUserPath expands a leading ~/ to the user home directory.
func ExpandUserPath(path string) string {
	if path == "~" {
		home, err := os.UserHomeDir()
		if err == nil {
			return home
		}
		return path
	}
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}

// LoadManagementIdentity loads the identity used to Identify() on a remote
// management link.
func LoadManagementIdentity(path string) (*identity.Identity, error) {
	path = ExpandUserPath(strings.TrimSpace(path))
	if path == "" {
		return nil, fmt.Errorf("management identity path required")
	}
	return identity.FromFile(path)
}

// EstablishRemoteManagementLink waits for a path to the remote instance's
// rnstransport.remote.management destination, opens a link, and Identifies
// with authIdentity.
func EstablishRemoteManagementLink(ctx context.Context, tr *transport.Transport, transportIdentityHash []byte, authIdentity *identity.Identity) (*link.Link, error) {
	if tr == nil {
		return nil, fmt.Errorf("nil transport")
	}
	if len(transportIdentityHash) != 16 {
		return nil, fmt.Errorf("transport identity hash must be 16 bytes")
	}
	if authIdentity == nil {
		return nil, fmt.Errorf("management identity required")
	}
	destHash := RemoteManagementDestHash(transportIdentityHash)
	if err := WaitPathWindow(ctx, tr, destHash); err != nil {
		return nil, fmt.Errorf("path: %w", err)
	}
	remote, err := identity.Recall(destHash)
	if err != nil {
		return nil, fmt.Errorf("recall: %w", err)
	}
	outDest, err := destination.New(remote, destination.Out, destination.Single, "rnstransport", tr, "remote", "management")
	if err != nil {
		return nil, err
	}
	l := link.NewLink(outDest, tr, nil, nil, nil)
	if err := activateOutboundLink(ctx, l); err != nil {
		return nil, fmt.Errorf("link: %w", err)
	}
	if err := l.Identify(authIdentity); err != nil {
		l.Teardown()
		return nil, fmt.Errorf("identify: %w", err)
	}
	return l, nil
}

// RemotePathRequest sends /path with command table or rates and waits for the
// response. destHash and maxHops may be nil.
func RemotePathRequest(ctx context.Context, l *link.Link, command string, destHash []byte, maxHops *int) (any, error) {
	if l == nil {
		return nil, fmt.Errorf("nil link")
	}
	timeout := 15 * time.Second
	if deadline, ok := ctx.Deadline(); ok {
		timeout = time.Until(deadline)
		if timeout <= 0 {
			return nil, ctx.Err()
		}
	}
	data := []any{command, destHash, nil}
	if maxHops != nil {
		data[2] = *maxHops
	}
	receipt, err := l.Request("/path", data, timeout)
	if err != nil {
		return nil, err
	}
	if err := WaitRequest(ctx, receipt); err != nil {
		return nil, err
	}
	if receipt.GetStatus() == link.StatusFailed {
		return nil, fmt.Errorf("remote request failed")
	}
	v := receipt.GetResponseValue()
	if v == nil {
		return nil, fmt.Errorf("remote request failed")
	}
	return v, nil
}

// RemoteStatusRequest sends /status with includeLinks and waits for the
// response list [stats] or [stats, link_count].
func RemoteStatusRequest(ctx context.Context, l *link.Link, includeLinks bool) (any, error) {
	if l == nil {
		return nil, fmt.Errorf("nil link")
	}
	timeout := 15 * time.Second
	if deadline, ok := ctx.Deadline(); ok {
		timeout = time.Until(deadline)
		if timeout <= 0 {
			return nil, ctx.Err()
		}
	}
	receipt, err := l.Request("/status", []any{includeLinks}, timeout)
	if err != nil {
		return nil, err
	}
	if err := WaitRequest(ctx, receipt); err != nil {
		return nil, err
	}
	if receipt.GetStatus() == link.StatusFailed {
		return nil, fmt.Errorf("remote request failed")
	}
	v := receipt.GetResponseValue()
	if v == nil {
		return nil, fmt.Errorf("remote request failed")
	}
	return v, nil
}

// PathTableFromResponse converts a remote /path table payload into entries.
func PathTableFromResponse(v any) []transport.PathTableEntry {
	switch x := v.(type) {
	case []transport.PathTableEntry:
		return x
	case []any:
		out := make([]transport.PathTableEntry, 0, len(x))
		for _, row := range x {
			if e, ok := pathTableEntryFromAny(row); ok {
				out = append(out, e)
			}
		}
		return out
	default:
		return nil
	}
}

// RateTableFromResponse converts a remote /path rates payload into entries.
func RateTableFromResponse(v any) []transport.RateTableEntry {
	switch x := v.(type) {
	case []transport.RateTableEntry:
		return x
	case []any:
		out := make([]transport.RateTableEntry, 0, len(x))
		for _, row := range x {
			if e, ok := rateTableEntryFromAny(row); ok {
				out = append(out, e)
			}
		}
		return out
	default:
		return nil
	}
}

// InterfaceStatsFromRemoteStatus unpacks /status response[0] into stats and
// an optional link count from response[1].
func InterfaceStatsFromRemoteStatus(v any) (transport.InterfaceStatsResponse, *int, error) {
	var stats transport.InterfaceStatsResponse
	list, ok := v.([]any)
	if !ok || len(list) == 0 {
		return stats, nil, fmt.Errorf("invalid remote status response")
	}
	packed, err := msgpack.Marshal(list[0])
	if err != nil {
		return stats, nil, err
	}
	if err := msgpack.Unmarshal(packed, &stats); err != nil {
		return stats, nil, err
	}
	if len(list) < 2 || list[1] == nil {
		return stats, nil, nil
	}
	n := int(asFloat64(list[1]))
	return stats, &n, nil
}

func pathTableEntryFromAny(v any) (transport.PathTableEntry, bool) {
	var e transport.PathTableEntry
	m := asStringMap(v)
	if m == nil {
		return e, false
	}
	e.Hash = asByteSlice(m["hash"])
	e.Via = asByteSlice(m["via"])
	e.Hops = uint8(asFloat64(m["hops"]))
	e.Expires = asFloat64(m["expires"])
	e.Timestamp = asFloat64(m["timestamp"])
	e.Interface = asStringVal(m["interface"])
	return e, len(e.Hash) > 0
}

func rateTableEntryFromAny(v any) (transport.RateTableEntry, bool) {
	var e transport.RateTableEntry
	m := asStringMap(v)
	if m == nil {
		return e, false
	}
	e.Hash = asByteSlice(m["hash"])
	e.Last = asFloat64(m["last"])
	e.RateViolations = int(asFloat64(m["rate_violations"]))
	e.BlockedUntil = asFloat64(m["blocked_until"])
	if ts, ok := m["timestamps"].([]any); ok {
		e.Timestamps = make([]float64, 0, len(ts))
		for _, x := range ts {
			e.Timestamps = append(e.Timestamps, asFloat64(x))
		}
	}
	return e, len(e.Hash) > 0
}

func asStringMap(v any) map[string]any {
	switch m := v.(type) {
	case map[string]any:
		return m
	case map[any]any:
		out := make(map[string]any, len(m))
		for k, val := range m {
			s, ok := k.(string)
			if !ok {
				continue
			}
			out[s] = val
		}
		return out
	default:
		return nil
	}
}
