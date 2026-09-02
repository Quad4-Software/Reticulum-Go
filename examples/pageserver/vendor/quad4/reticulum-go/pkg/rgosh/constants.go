// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package rgosh

import "time"

const (
	// ProtocolVersion is the native rgosh protocol version.
	ProtocolVersion = 1

	// CompatProtocolVersion matches Python rnsh PROTOCOL_VERSION.
	CompatProtocolVersion = 1

	MsgMagicNative = 0xad
	MsgMagicCompat = 0xac

	StreamStdin  = 0
	StreamStdout = 1
	StreamStderr = 2

	MaxStreamChunk  = 16 * 1024
	MaxDecompressed = 16 * 1024
	CompressThresh  = 32
	MaxArgvLen      = 256
	MaxArgBytes     = 64 * 1024
	MaxTermLen      = 256
	MaxErrorLen     = 4 * 1024
	MaxSoftwareLen  = 128

	CapLineMode uint16 = 1 << 0
	CapCoalesce uint16 = 1 << 1
	CapResource uint16 = 1 << 2

	DefaultSoftware = "rgosh"

	// AutoLineRTT is the RTT above which line mode is preferred.
	AutoLineRTT = 750 * time.Millisecond

	DefaultCoalesceWindow = 40 * time.Millisecond

	StreamHeaderSize   = 2
	CompressionTries   = 6
	MaxStdinPending    = 64 * 1024
	DrainAfterExit     = 5 * time.Second
	DefaultAnnounceSec = 900
)

// Native message types: 0xad00 | n
const (
	NativeNoop       uint16 = 0xad00
	NativeWinSize    uint16 = 0xad02
	NativeExec       uint16 = 0xad03
	NativeStream     uint16 = 0xad04
	NativeVersion    uint16 = 0xad05
	NativeError      uint16 = 0xad06
	NativeExit       uint16 = 0xad07
	NativeAuthOK     uint16 = 0xad08
	NativeAuthDenied uint16 = 0xad09
)

// Compat message types: 0xac00 | n (Python rnsh)
const (
	CompatNoop    uint16 = 0xac00
	CompatWinSize uint16 = 0xac02
	CompatExec    uint16 = 0xac03
	CompatStream  uint16 = 0xac04
	CompatVersion uint16 = 0xac05
	CompatError   uint16 = 0xac06
	CompatExit    uint16 = 0xac07
)

// State is the session finite state machine.
type State int

const (
	StateWaitIdent State = iota
	StateWaitVers
	StateWaitCmd
	StateRunning
	StateError
	StateTeardown
)

func (s State) String() string {
	switch s {
	case StateWaitIdent:
		return "WAIT_IDENT"
	case StateWaitVers:
		return "WAIT_VERS"
	case StateWaitCmd:
		return "WAIT_CMD"
	case StateRunning:
		return "RUNNING"
	case StateError:
		return "ERROR"
	case StateTeardown:
		return "TEARDOWN"
	default:
		return "UNKNOWN"
	}
}
