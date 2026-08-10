// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

//go:build !unix

package cli

import (
	"os"
	"os/signal"
	"syscall"
)

func notifyShellSignals(sig chan os.Signal) {
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
}

func shellSignalWinch(os.Signal, func()) bool {
	return false
}
