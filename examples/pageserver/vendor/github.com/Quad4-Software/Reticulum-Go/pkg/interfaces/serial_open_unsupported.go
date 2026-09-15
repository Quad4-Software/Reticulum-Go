// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

//go:build !js && !(linux || darwin || freebsd || openbsd || windows)

package interfaces

import (
	"fmt"
	"runtime"
)

// openHardwareSerial is unavailable because go.bug.st/serial does not support
// this GOOS (for example NetBSD or Dragonfly). Use an injected Open or a
// TCP-backed RNode port instead.
func openHardwareSerial(opts SerialOptions) (SerialPort, error) {
	return nil, fmt.Errorf("native serial open is not available on %s/%s", runtime.GOOS, runtime.GOARCH)
}
