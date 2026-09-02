// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package discovery

import (
	"crypto/sha256"
	"fmt"
	"net"
	"strings"
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

// isSafeDiscoveryPersistName accepts only lowercase-hex SHA-256 hashes.
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
