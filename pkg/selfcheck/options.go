// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package selfcheck

import "time"

// Options configures which self-check tiers run.
type Options struct {
	// Quick runs core and platform tiers only.
	Quick bool
	// Full enables optional interface probes (QUIC, HTTPS, VSOCK, Pipe, Serial).
	Full bool
	// Interop runs optional external checks when tools are present.
	Interop bool
	// Strict promotes warnings to failures at exit-code time (CLI uses Report.ExitCode).
	Strict bool
	// BinaryPath is the reticulum-go binary for daemon and CLI checks.
	// Empty skips those checks or uses os.Executable when it is the CLI binary.
	BinaryPath string
	// Timeout bounds network and daemon waits.
	Timeout time.Duration
	// SkipDaemon skips Tier D even when a binary is available.
	SkipDaemon bool
	// WorkDir is an optional parent for temp directories. Empty uses os.TempDir.
	WorkDir string
}

func (o Options) timeout() time.Duration {
	if o.Timeout > 0 {
		return o.Timeout
	}
	return defaultTimeout
}
