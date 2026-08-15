// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package common

import (
	"runtime"
	"strings"
)

// DefaultSharedInstanceType returns the platform shared-instance transport.
// Matches Python RNS: abstract Unix on Linux, TCP elsewhere.
func DefaultSharedInstanceType() string {
	if runtime.GOOS == "linux" {
		return SharedInstanceUnix
	}
	return SharedInstanceTCP
}

// ResolveSharedInstanceType returns typ when set to tcp or unix, otherwise the
// platform default. Invalid values fall back to the platform default.
func ResolveSharedInstanceType(typ string) string {
	switch strings.ToLower(strings.TrimSpace(typ)) {
	case SharedInstanceTCP, SharedInstanceUnix:
		return strings.ToLower(strings.TrimSpace(typ))
	default:
		return DefaultSharedInstanceType()
	}
}

// SharedInstanceUsesUnix reports whether typ resolves to Unix sockets.
func SharedInstanceUsesUnix(typ string) bool {
	return ResolveSharedInstanceType(typ) == SharedInstanceUnix
}
