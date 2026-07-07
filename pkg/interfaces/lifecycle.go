// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package interfaces

import (
	"runtime"

	"quad4/reticulum-go/pkg/debug"
)

// ConnectivityNotifier allows embedders to observe interface up/down transitions.
type ConnectivityNotifier interface {
	SetConnectivityHooks(onDown, onUp func())
}

// NormalizeMaxReconnectTries maps config values to an attempt limit.
// Zero means unlimited, Negative values also mean unlimited. Positive values cap reconnect attempts.
func NormalizeMaxReconnectTries(cfgValue int) int {
	if cfgValue <= 0 {
		return -1
	}
	return cfgValue
}

// applyClientTCPTimeouts configures keepalive and user timeout on a dialed connection.
func applyClientTCPTimeouts(tc *TCPClientInterface) {
	if tc == nil || tc.conn == nil {
		return
	}
	switch runtime.GOOS {
	case "linux", "android":
		if err := tc.setTimeoutsLinux(); err != nil {
			debug.Log(debug.DebugError, "Failed to set Linux TCP timeouts", "error", err)
		}
	case "darwin":
		if err := tc.setTimeoutsOSX(); err != nil {
			debug.Log(debug.DebugError, "Failed to set OSX TCP timeouts", "error", err)
		}
	}
}
