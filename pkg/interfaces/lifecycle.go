// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package interfaces

import (
	"github.com/Quad4-Software/Reticulum-Go/pkg/common"
	"runtime"

	"github.com/Quad4-Software/Reticulum-Go/pkg/debug"
)

// ConnectivityNotifier allows embedders to observe interface up/down transitions.
type ConnectivityNotifier interface {
	SetConnectivityHooks(onDown, onUp func())
}

// ReconnectNever is the sentinel for an explicit max_reconnect_tries = 0
// in config: the link must not reconnect. It survives normalization where
// other non-positive values map to unlimited.
const ReconnectNever = -2

// NormalizeMaxReconnectTries maps config values to an attempt limit.
// Zero means unlimited, Negative values also mean unlimited. Positive values cap reconnect attempts.
func NormalizeMaxReconnectTries(cfgValue int) int {
	if cfgValue == ReconnectNever {
		return ReconnectNever
	}
	if cfgValue <= 0 {
		return -1
	}
	return cfgValue
}

// MaxReconnectTriesFromConfig resolves the configured value honoring the
// absent-vs-zero distinction: unset or negative is unlimited, explicit 0
// means never reconnect, positive caps attempts.
func MaxReconnectTriesFromConfig(cfg *common.InterfaceConfig) int {
	if cfg == nil {
		return -1
	}
	if cfg.MaxReconnTriesSet && cfg.MaxReconnTries == 0 {
		return ReconnectNever
	}
	return NormalizeMaxReconnectTries(cfg.MaxReconnTries)
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
