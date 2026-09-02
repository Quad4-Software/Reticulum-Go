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
	"quad4/reticulum-go/pkg/blackhole"
	"quad4/reticulum-go/pkg/destination"
	"quad4/reticulum-go/pkg/identity"
	"quad4/reticulum-go/pkg/link"
	"quad4/reticulum-go/pkg/profiler"
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
	timeout := time.Duration(0)
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

// RemoteStatusRequest sends /status with [includeLinks, includeProfiling]
// matching Python rnstatus remote queries. Response is
// [stats], [stats, link_count], [stats, profiling], or
// [stats, link_count, profiling].
func RemoteStatusRequest(ctx context.Context, l *link.Link, includeLinks, includeProfiling bool) (any, error) {
	if l == nil {
		return nil, fmt.Errorf("nil link")
	}
	timeout := time.Duration(0)
	if deadline, ok := ctx.Deadline(); ok {
		timeout = time.Until(deadline)
		if timeout <= 0 {
			return nil, ctx.Err()
		}
	}
	receipt, err := l.Request("/status", []any{includeLinks, includeProfiling}, timeout)
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

// InterfaceStatsFromRemoteStatus unpacks a Python-compatible /status response.
// includeLinks / includeProfiling must match the request flags so optional
// trailing fields are interpreted correctly.
func InterfaceStatsFromRemoteStatus(v any, includeLinks, includeProfiling bool) (transport.InterfaceStatsResponse, *int, string, error) {
	var stats transport.InterfaceStatsResponse
	list, ok := v.([]any)
	if !ok || len(list) == 0 {
		return stats, nil, "", fmt.Errorf("invalid remote status response")
	}
	packed, err := msgpack.Marshal(list[0])
	if err != nil {
		return stats, nil, "", err
	}
	if err := msgpack.Unmarshal(packed, &stats); err != nil {
		return stats, nil, "", err
	}
	idx := 1
	var linkCount *int
	if includeLinks && len(list) > idx {
		n := int(asFloat64(list[idx]))
		linkCount = &n
		idx++
	}
	profilingText := ""
	if includeProfiling && len(list) > idx && list[idx] != nil {
		profilingText = formatProfilingResults(list[idx])
	}
	return stats, linkCount, profilingText, nil
}

func formatProfilingResults(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case []byte:
		return string(x)
	case map[string]profiler.TagResult:
		return profiler.FormatResults(x)
	case map[string]any:
		// Python rnsd returns nested maps. Format a compact summary.
		if len(x) == 0 {
			return ""
		}
		converted := make(map[string]profiler.TagResult, len(x))
		for k, raw := range x {
			tr, ok := tagResultFromAny(raw)
			if !ok {
				continue
			}
			converted[k] = tr
		}
		if len(converted) == 0 {
			b, err := msgpack.Marshal(x)
			if err != nil {
				return fmt.Sprintf("%v", x)
			}
			return string(b)
		}
		return profiler.FormatResults(converted)
	default:
		return fmt.Sprintf("%v", x)
	}
}

func tagResultFromAny(v any) (profiler.TagResult, bool) {
	m := asStringMap(v)
	if m == nil {
		return profiler.TagResult{}, false
	}
	tr := profiler.TagResult{Name: asStringVal(m["name"])}
	if tr.Name == "" {
		return profiler.TagResult{}, false
	}
	if s, ok := m["super"].(string); ok {
		tr.Super = &s
	}
	tr.StatsAll = statsFromAny(m["stats_all"])
	tr.Stats1m = statsFromAny(m["stats_1m"])
	tr.Stats5m = statsFromAny(m["stats_5m"])
	tr.Stats30m = statsFromAny(m["stats_30m"])
	tr.Stats60m = statsFromAny(m["stats_60m"])
	return tr, true
}

func statsFromAny(v any) *profiler.Stats {
	m := asStringMap(v)
	if m == nil {
		return nil
	}
	st := &profiler.Stats{Count: int(asFloat64(m["count"]))}
	if st.Count == 0 {
		return st
	}
	mean := asFloat64(m["mean"])
	med := asFloat64(m["median"])
	minV := asFloat64(m["min"])
	maxV := asFloat64(m["max"])
	sum := asFloat64(m["sum"])
	st.Mean, st.Median, st.Min, st.Max, st.Sum = &mean, &med, &minV, &maxV, &sum
	if _, ok := m["stdev"]; ok && m["stdev"] != nil {
		s := asFloat64(m["stdev"])
		st.Stdev = &s
	}
	if _, ok := m["threads"]; ok && m["threads"] != nil {
		t := int(asFloat64(m["threads"]))
		st.Threads = &t
	}
	return st
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

const (
	blackholeListName = "rnstransport.info.blackhole"
	blackholeListPath = "/list"
)

// FetchPublishedBlackholeList opens a link to rnstransport.info.blackhole for
// transportIdentityHash and requests /list (Python rnpath -p).
func FetchPublishedBlackholeList(ctx context.Context, tr *transport.Transport, transportIdentityHash []byte) ([]BlackholeEntry, error) {
	if tr == nil {
		return nil, fmt.Errorf("nil transport")
	}
	if len(transportIdentityHash) != 16 {
		return nil, fmt.Errorf("transport identity hash must be 16 bytes")
	}
	destHash := destination.HashFromNameAndIdentity(blackholeListName, transportIdentityHash)
	if err := WaitPath(ctx, tr, destHash); err != nil {
		return nil, err
	}
	remote, err := identity.Recall(destHash)
	if err != nil {
		return nil, fmt.Errorf("recall: %w", err)
	}
	outDest, err := destination.New(remote, destination.Out, destination.Single, "rnstransport", tr, "info", "blackhole")
	if err != nil {
		return nil, err
	}
	established := make(chan struct{}, 1)
	l := link.NewLink(outDest, tr, nil, func(*link.Link) {
		select {
		case established <- struct{}{}:
		default:
		}
	}, nil)
	if err := l.Establish(); err != nil {
		return nil, fmt.Errorf("establish: %w", err)
	}
	defer l.Teardown()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-established:
	case <-time.After(45 * time.Second):
		return nil, fmt.Errorf("link establish timeout")
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) && l.GetStatus() != link.StatusActive {
		time.Sleep(50 * time.Millisecond)
	}
	timeout := time.Duration(0)
	if d, ok := ctx.Deadline(); ok {
		timeout = time.Until(d)
		if timeout <= 0 {
			return nil, ctx.Err()
		}
	}
	receipt, err := l.Request(blackholeListPath, nil, timeout)
	if err != nil {
		return nil, err
	}
	if err := WaitRequest(ctx, receipt); err != nil {
		return nil, err
	}
	if receipt.GetStatus() == link.StatusFailed {
		return nil, fmt.Errorf("remote request failed")
	}
	raw := receipt.GetResponse()
	if len(raw) == 0 {
		return nil, nil
	}
	decoded, err := blackhole.DecodeBlackholeMap(raw)
	if err != nil {
		return nil, err
	}
	out := make([]BlackholeEntry, 0, len(decoded))
	for idStr, e := range decoded {
		out = append(out, BlackholeEntry{
			Identity: []byte(idStr),
			Until:    e.Until,
			Reason:   e.Reason,
			Source:   append([]byte(nil), e.Source...),
		})
	}
	return out, nil
}
