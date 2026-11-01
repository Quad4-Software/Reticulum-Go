// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package interfaces

import (
	"net"
	"runtime"
)

// usableNetworkAddrs returns the set of global-unicast addresses on up,
// running, non-loopback interfaces, keyed by interface name and address. A
// gained key means an interface came up. An interface whose addresses
// cannot be listed is skipped.
func usableNetworkAddrs() (map[string]struct{}, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	out := make(map[string]struct{})
	for i := range ifaces {
		iface := &ifaces[i]
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		// Go never sets FlagRunning on Windows.
		if iface.Flags&net.FlagRunning == 0 && runtime.GOOS != "windows" {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ipnet, ok := addr.(*net.IPNet)
			if !ok || !ipnet.IP.IsGlobalUnicast() {
				continue
			}
			out[iface.Name+"|"+ipnet.IP.String()] = struct{}{}
		}
	}
	return out, nil
}
