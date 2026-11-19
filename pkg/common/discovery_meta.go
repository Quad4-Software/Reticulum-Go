// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package common

import (
	"runtime/debug"
	"strings"
)

// ImplementationName is the short transport stack id announced in discovery
// info (RNS 1.5.1+ TRANSPORT_IMPL) and tunnel synthesis metadata (1.6.1).
// Python RNS uses "RNS".
const ImplementationName = "reticulum-go"

// implementationVersion is the fallback TRANSPORT_VERS when build info is
// unavailable. Link with -X if a pinned release string is required.
var implementationVersion = "dev"

// ImplementationVersion returns the transport version string for discovery
// announces (RNS 1.5.1+ TRANSPORT_VERS) and tunnel synthesis metadata.
func ImplementationVersion() string {
	if implementationVersion != "" && implementationVersion != "dev" {
		return implementationVersion
	}
	if info, ok := debug.ReadBuildInfo(); ok {
		if v := info.Main.Version; v != "" && v != "(devel)" {
			return v
		}
	}
	if implementationVersion == "" {
		return "dev"
	}
	return implementationVersion
}

// VersionTuple parses a dotted version string into integer components,
// mirroring Python Discovery.version_tuple. Each dot-separated component
// contributes its leading digits; the first component without leading digits
// stops the parse. A leading v or V is stripped so release tags like v1.3.0
// compare like Python's bare "1.5.2" strings. Returns nil when no digits
// parse.
func VersionTuple(version string) []int {
	version = strings.TrimSpace(version)
	if version == "" {
		return nil
	}
	version = strings.TrimPrefix(version, "v")
	version = strings.TrimPrefix(version, "V")
	var out []int
	for _, component := range strings.Split(version, ".") {
		i := 0
		for i < len(component) && component[i] >= '0' && component[i] <= '9' {
			i++
		}
		if i == 0 {
			break
		}
		n := 0
		for _, c := range component[:i] {
			n = n*10 + int(c-'0')
		}
		out = append(out, n)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// VersionAtLeast reports whether version parses to at least minimum, using
// VersionTuple semantics. Missing trailing components compare as zero, like
// Python tuple comparison (1,5) < (1,5,2) and (1,6) >= (1,5,2).
func VersionAtLeast(version, minimum string) bool {
	v := VersionTuple(version)
	m := VersionTuple(minimum)
	if v == nil || m == nil {
		return false
	}
	for i := 0; i < len(m); i++ {
		var vc int
		if i < len(v) {
			vc = v[i]
		}
		if vc > m[i] {
			return true
		}
		if vc < m[i] {
			return false
		}
	}
	return true
}

// SanitizeDiscoveryName matches Python InterfaceAnnounceHandler.sanitize_name:
// ASCII alnum edges, strip CR/LF, collapse multi-spaces.
func SanitizeDiscoveryName(in string) string {
	if in == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(in))
	for _, r := range in {
		if r == '\n' || r == '\r' {
			continue
		}
		if r > 127 {
			continue
		}
		b.WriteRune(r)
	}
	out := strings.TrimSpace(b.String())
	for _, n := range []int{5, 3, 2} {
		out = strings.ReplaceAll(out, strings.Repeat(" ", n), " ")
	}
	for len(out) > 0 && !isDiscoverySanMap(out[0]) {
		out = out[1:]
	}
	for len(out) > 0 {
		last := out[len(out)-1]
		if isDiscoverySanMap(last) || last == ')' {
			break
		}
		out = out[:len(out)-1]
	}
	const maxDiscoveryName = 255
	if len(out) > maxDiscoveryName {
		out = out[:maxDiscoveryName]
	}
	return out
}

func isDiscoverySanMap(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z')
}
