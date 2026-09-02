// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package debug

// Debug log levels. Higher values emit more detail.
// Numeric values match Python RNS at the default (4 = info) so a shared
// config with loglevel = 4 is quiet enough for production.
// 0 silences all output (Go-only, used by tests).
const (
	// DebugCritical is fatal conditions (process cannot usefully continue).
	DebugCritical = 1
	// DebugError is a failed operation.
	DebugError = 2
	// DebugWarning is unexpected but recovered.
	DebugWarning = 3
	// DebugInfo is operator-facing lifecycle (start, stop, interface up, link up).
	DebugInfo = 4
	// DebugVerbose is per-session protocol detail.
	DebugVerbose = 5
	// DebugTrace is per-packet forwarding and header fields.
	DebugTrace = 6
	// DebugPackets is wire dumps and packet hex.
	DebugPackets = 7
	// DebugAll is an alias for DebugPackets.
	DebugAll = DebugPackets
)

// LevelName returns a short label for an RNS debug level.
func LevelName(level int) string {
	switch level {
	case DebugCritical:
		return "critical"
	case DebugError:
		return "error"
	case DebugWarning:
		return "warning"
	case DebugInfo:
		return "info"
	case DebugVerbose:
		return "verbose"
	case DebugTrace:
		return "trace"
	case DebugPackets:
		return "packets"
	default:
		if level <= 0 {
			return "silent"
		}
		return "unknown"
	}
}

// ClampLevel bounds n to the silent-through-packets range.
func ClampLevel(n int) int {
	if n < 0 {
		return 0
	}
	if n > DebugAll {
		return DebugAll
	}
	return n
}
