// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

//go:build !windows && !js

package rgosh

import (
	"os"
	"syscall"
)

// waitStatusCode maps a signaled exit onto Python rnsh's "waitpid & 0xff"
// exit-code contract: the signal number plus 128 when the child dumped core.
// Go's ExitCode reports -1 for signal deaths. A remote Python rnsh mirrors
// that as process exit status 255.
func waitStatusCode(state *os.ProcessState, fallback int) int {
	if ws, ok := state.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
		code := int(ws.Signal())
		if ws.CoreDump() {
			code += 128
		}
		return code
	}
	return fallback
}
