// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package discovery

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"

	"quad4/msgpack/v5/pkg/msgpack"
)

// AutoconnectTypes lists announced interface types that may be auto-connected.
// Matching Python InterfaceDiscovery.AUTOCONNECT_TYPES.
var AutoconnectTypes = map[string]struct{}{
	"BackboneInterface":  {},
	"TCPServerInterface": {},
	"I2PInterface":       {},
}

// EndpointHash returns the SHA-256 of "host:port" used to dedupe autoconnects.
func EndpointHash(info *ReceivedAnnounceInfo) []byte {
	if info == nil {
		return nil
	}
	spec := info.Info.ReachableOn
	if info.Info.HasPort {
		spec += ":" + fmt.Sprintf("%d", info.Info.Port)
	}
	sum := sha256.Sum256([]byte(spec))
	return sum[:]
}

// IsYggIPv6 reports whether addr is in Yggdrasil 200::/7.
func IsYggIPv6(addr string) bool {
	ip := net.ParseIP(strings.TrimSpace(addr))
	if ip == nil {
		return false
	}
	ip = ip.To16()
	if ip == nil {
		return false
	}
	return ip[0]&0xfe == 0x02
}

// PersistDiscoveredInterface writes info under storageDir/discovery/interfaces/.
func PersistDiscoveredInterface(storageDir string, info *ReceivedAnnounceInfo) error {
	if storageDir == "" || info == nil {
		return nil
	}
	dir := filepath.Join(storageDir, "discovery", "interfaces")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}
	eh := EndpointHash(info)
	if len(eh) == 0 {
		return nil
	}
	payload := map[string]any{
		"type":         info.Info.Type,
		"name":         info.Info.Name,
		"reachable_on": info.Info.ReachableOn,
		"port":         info.Info.Port,
		"transport_id": info.Info.TransportID,
		"network_id":   info.RemoteIdentity,
		"ifac_netname": info.Info.IFACNetname,
		"ifac_netkey":  info.Info.IFACNetkey,
		"transport":    info.Info.Transport,
	}
	packed, err := msgpack.Marshal(payload)
	if err != nil {
		return err
	}
	path := filepath.Join(dir, hex.EncodeToString(eh))
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, packed, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// LoadPersistedInterfaces reads previously discovered interface announces.
func LoadPersistedInterfaces(storageDir string) ([]*ReceivedAnnounceInfo, error) {
	if storageDir == "" {
		return nil, nil
	}
	dir := filepath.Join(storageDir, "discovery", "interfaces")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	out := make([]*ReceivedAnnounceInfo, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !isSafeDiscoveryPersistName(name) {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, name)) // #nosec G304 -- name validated by isSafeDiscoveryPersistName
		if err != nil {
			continue
		}
		var m map[string]any
		if err := msgpack.Unmarshal(raw, &m); err != nil {
			continue
		}
		info := receivedFromPersistMap(m)
		if info != nil {
			out = append(out, info)
		}
	}
	return out, nil
}

func receivedFromPersistMap(m map[string]any) *ReceivedAnnounceInfo {
	if m == nil {
		return nil
	}
	info := &ReceivedAnnounceInfo{}
	info.Info.Type, _ = m["type"].(string)
	info.Info.Name, _ = m["name"].(string)
	info.Info.ReachableOn, _ = m["reachable_on"].(string)
	info.Info.IFACNetname, _ = m["ifac_netname"].(string)
	info.Info.IFACNetkey, _ = m["ifac_netkey"].(string)
	if v, ok := m["transport"].(bool); ok {
		info.Info.Transport = v
	}
	switch p := m["port"].(type) {
	case int64:
		info.Info.Port = p
		info.Info.HasPort = true
	case uint64:
		if p > uint64(maxInt64) {
			info.Info.Port = maxInt64
		} else {
			info.Info.Port = int64(p) //nolint:gosec // G115: guarded above
		}
		info.Info.HasPort = true
	case int:
		info.Info.Port = int64(p)
		info.Info.HasPort = true
	case float64:
		info.Info.Port = int64(p)
		info.Info.HasPort = true
	}
	info.Info.TransportID = asBytes(m["transport_id"])
	info.RemoteIdentity = asBytes(m["network_id"])
	if info.Info.Type == "" || info.Info.ReachableOn == "" {
		return nil
	}
	return info
}

func asBytes(v any) []byte {
	switch x := v.(type) {
	case []byte:
		return append([]byte(nil), x...)
	case string:
		return []byte(x)
	default:
		return nil
	}
}

const maxInt64 = int64(1<<63 - 1)

// isSafeDiscoveryPersistName accepts only lowercase-hex SHA-256 endpoint hashes.
func isSafeDiscoveryPersistName(name string) bool {
	if len(name) != 64 {
		return false
	}
	for i := 0; i < len(name); i++ {
		c := name[i]
		switch {
		case c >= '0' && c <= '9':
		case c >= 'a' && c <= 'f':
		default:
			return false
		}
	}
	return true
}
