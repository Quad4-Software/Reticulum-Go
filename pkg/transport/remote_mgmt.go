// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package transport

import (
	"bytes"
	"fmt"
	"math"
	"time"

	"quad4/msgpack/v5/pkg/msgpack"
	"quad4/reticulum-go/pkg/debug"
	"quad4/reticulum-go/pkg/destination"
	"quad4/reticulum-go/pkg/identity"
)

const (
	remoteManagementApp      = "rnstransport"
	remoteManagementAspectA  = "remote"
	remoteManagementAspectB  = "management"
	remoteManagementPathPath = "/path"
	remoteManagementPathStat = "/status"
)

// InitializeRemoteManagement registers rnstransport.remote.management with
// /path and /status request handlers when enable_remote_management is set
// and this process owns the transport (not a shared-instance client).
func (t *Transport) InitializeRemoteManagement() error {
	if t == nil || t.config == nil || !t.config.EnableRemoteManagement {
		return nil
	}
	if t.ConnectedToSharedInstance() {
		return nil
	}
	if t.transportIdentity == nil {
		return fmt.Errorf("transport identity not initialized")
	}
	if t.remoteManagementDest != nil {
		return nil
	}

	dest, err := destination.New(t.transportIdentity, destination.In, destination.Single, remoteManagementApp, t, remoteManagementAspectA, remoteManagementAspectB)
	if err != nil {
		return fmt.Errorf("remote management destination: %w", err)
	}
	dest.AcceptsLinks(true)

	allowed := copyHashList(t.config.RemoteManagementAllowed)
	if len(allowed) == 0 {
		debug.Log(debug.DebugCritical, "Remote management enabled with empty ACL, request handlers not registered")
	} else {
		if err := dest.RegisterRequestHandlerAny(remoteManagementPathPath, t.remotePathHandler, destination.AllowList, allowed); err != nil {
			return fmt.Errorf("remote /path handler: %w", err)
		}
		if err := dest.RegisterRequestHandlerAny(remoteManagementPathStat, t.remoteStatusHandler, destination.AllowList, allowed); err != nil {
			return fmt.Errorf("remote /status handler: %w", err)
		}
	}

	t.RegisterDestination(dest.GetHash(), dest)
	t.mutex.Lock()
	t.remoteManagementDest = dest
	t.mgmtDestinations = append(t.mgmtDestinations, dest)
	t.mutex.Unlock()

	debug.Log(debug.DebugCritical, "Enabled remote management",
		"destination", fmt.Sprintf("%x", dest.GetHash()))
	return nil
}

// RemoteManagementDestination returns the local remote-management destination.
func (t *Transport) RemoteManagementDestination() *destination.Destination {
	if t == nil {
		return nil
	}
	t.mutex.RLock()
	defer t.mutex.RUnlock()
	return t.remoteManagementDest
}

func (t *Transport) appendMgmtDestination(d *destination.Destination) {
	if t == nil || d == nil {
		return
	}
	t.mutex.Lock()
	defer t.mutex.Unlock()
	t.mgmtDestinations = append(t.mgmtDestinations, d)
}

func (t *Transport) maybeAnnounceMgmtDestinations() {
	t.mutex.Lock()
	if time.Since(t.lastMgmtAnnounce) < MgmtAnnounceInterval {
		t.mutex.Unlock()
		return
	}
	t.lastMgmtAnnounce = time.Now()
	dests := append([]*destination.Destination(nil), t.mgmtDestinations...)
	t.mutex.Unlock()
	for _, d := range dests {
		if d == nil {
			continue
		}
		_ = d.Announce(false, nil, nil)
	}
}

func (t *Transport) remotePathHandler(_ string, data []byte, _ []byte, _ []byte, remote *identity.Identity, _ int64) any {
	if remote == nil {
		return nil
	}
	list := unpackRequestList(data)
	if len(list) == 0 {
		return nil
	}
	command := asStringAny(list[0])
	var destHash []byte
	if len(list) > 1 {
		destHash = asBytesAny(list[1])
	}
	var maxHops *int
	if len(list) > 2 {
		maxHops = asIntPtrAny(list[2])
	}

	switch command {
	case "table":
		table := t.GetPathTable(maxHops)
		if len(destHash) == 0 {
			return table
		}
		out := make([]PathTableEntry, 0)
		for _, e := range table {
			if bytes.Equal(e.Hash, destHash) {
				out = append(out, e)
			}
		}
		return out
	case "rates":
		table := t.GetRateTableRPC()
		if table == nil {
			table = []RateTableEntry{}
		}
		if len(destHash) == 0 {
			return table
		}
		out := make([]RateTableEntry, 0)
		for _, e := range table {
			if bytes.Equal(e.Hash, destHash) {
				out = append(out, e)
			}
		}
		return out
	default:
		return nil
	}
}

func (t *Transport) remoteStatusHandler(_ string, data []byte, _ []byte, _ []byte, remote *identity.Identity, _ int64) any {
	if remote == nil {
		return nil
	}
	list := unpackRequestList(data)
	if len(list) == 0 {
		return nil
	}
	out := []any{t.GetInterfaceStatsRPC()}
	if asBoolAny(list[0]) {
		out = append(out, t.GetLinkCountRPC())
	}
	return out
}

func unpackRequestList(data []byte) []any {
	if len(data) == 0 {
		return nil
	}
	var out []any
	if err := msgpack.Unmarshal(data, &out); err != nil {
		return nil
	}
	return out
}

func copyHashList(in [][]byte) [][]byte {
	if len(in) == 0 {
		return nil
	}
	out := make([][]byte, len(in))
	for i, h := range in {
		out[i] = append([]byte(nil), h...)
	}
	return out
}

func asStringAny(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case []byte:
		return string(x)
	default:
		return ""
	}
}

func asBytesAny(v any) []byte {
	switch x := v.(type) {
	case []byte:
		return x
	case string:
		return []byte(x)
	default:
		return nil
	}
}

func asIntPtrAny(v any) *int {
	if v == nil {
		return nil
	}
	switch n := v.(type) {
	case int:
		return &n
	case int8:
		i := int(n)
		return &i
	case int16:
		i := int(n)
		return &i
	case int32:
		i := int(n)
		return &i
	case int64:
		if n > int64(math.MaxInt) || n < int64(math.MinInt) {
			return nil
		}
		i := int(n)
		return &i
	case uint:
		if n > math.MaxInt {
			return nil
		}
		i := int(n)
		return &i
	case uint8:
		i := int(n)
		return &i
	case uint16:
		i := int(n)
		return &i
	case uint32:
		i := int(n)
		return &i
	case uint64:
		if n > uint64(math.MaxInt) {
			return nil
		}
		i := int(n)
		return &i
	case float32:
		i := int(n)
		return &i
	case float64:
		i := int(n)
		return &i
	default:
		return nil
	}
}

func asBoolAny(v any) bool {
	b, ok := v.(bool)
	return ok && b
}
