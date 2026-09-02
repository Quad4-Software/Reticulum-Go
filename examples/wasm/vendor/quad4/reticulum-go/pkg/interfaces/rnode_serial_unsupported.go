// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

//go:build !js && !(linux || darwin || freebsd || openbsd || windows)

package interfaces

import (
	"fmt"
	"runtime"
	"strings"
)

func openRNodeSerial(opts SerialOptions) (SerialPort, error) {
	port := strings.TrimSpace(opts.Device)
	if strings.HasPrefix(strings.ToLower(port), "tcp://") {
		return nil, fmt.Errorf("RNode TCP open is handled by RNodeInterface")
	}
	return nil, fmt.Errorf("RNode native serial is not available on %s/%s use tcp://host:port or inject Open", runtime.GOOS, runtime.GOARCH)
}
