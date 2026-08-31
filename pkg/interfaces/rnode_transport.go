// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

//go:build !js

package interfaces

import (
	"fmt"
	"strings"
	"sync"
)

// RNodePortOpener opens an RNode transport for a URI target after the scheme
// prefix is stripped. Example: for ble://AA:BB:CC:DD:EE:FF the target is the
// address string. For usb:///dev/bus/usb/001/002 the target is the path.
type RNodePortOpener func(target string) (SerialPort, error)

var (
	rnodeOpenerMu sync.RWMutex
	rnodeOpeners  = map[string]RNodePortOpener{}
)

// RegisterRNodePortOpener installs a scheme opener used by RNodeInterface when
// Port uses that scheme (ble, bt, usb). On Android, host apps register USB
// Host and BLE UART openers that return a SerialPort (often an RNodeHostPipe).
// Passing a nil opener removes the scheme.
func RegisterRNodePortOpener(scheme string, opener RNodePortOpener) {
	scheme = strings.ToLower(strings.TrimSpace(scheme))
	scheme = strings.TrimSuffix(scheme, "://")
	if scheme == "" {
		return
	}
	rnodeOpenerMu.Lock()
	defer rnodeOpenerMu.Unlock()
	if opener == nil {
		delete(rnodeOpeners, scheme)
		return
	}
	rnodeOpeners[scheme] = opener
}

func lookupRNodePortOpener(scheme string) RNodePortOpener {
	rnodeOpenerMu.RLock()
	defer rnodeOpenerMu.RUnlock()
	return rnodeOpeners[strings.ToLower(scheme)]
}

func parseRNodePortURI(raw string) (scheme, target string, ok bool) {
	raw = strings.TrimSpace(raw)
	lower := strings.ToLower(raw)
	for _, s := range []string{"ble://", "bt://", "usb://"} {
		if strings.HasPrefix(lower, s) {
			return strings.TrimSuffix(s, "://"), raw[len(s):], true
		}
	}
	return "", raw, false
}

func openRNodeRegisteredPort(port string) (SerialPort, error) {
	scheme, target, isURI := parseRNodePortURI(port)
	if !isURI {
		return nil, fmt.Errorf("not a registered RNode URI")
	}
	opener := lookupRNodePortOpener(scheme)
	if opener == nil {
		return nil, fmt.Errorf("RNode %s transport is not registered (Android host must RegisterRNodePortOpener(%q, ...))", scheme, scheme)
	}
	return opener(target)
}
