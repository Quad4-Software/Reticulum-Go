// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package interfaces

import (
	"net"
	"time"

	"quad4/reticulum-go/pkg/debug"
)

const dataListenerRetryDelay = 1250 * time.Millisecond

func (ai *AutoInterface) currentLinkLocalAddr(ifname string) string {
	iface, err := net.InterfaceByName(ifname)
	if err != nil {
		return ""
	}
	addrs, err := iface.Addrs()
	if err != nil {
		return ""
	}
	for _, addr := range addrs {
		ipnet, ok := addr.(*net.IPNet)
		if !ok || ipnet.IP.To4() != nil || !ipnet.IP.IsLinkLocalUnicast() {
			continue
		}
		return descopeLinkLocal(ipnet.IP.String())
	}
	return ""
}

func (ai *AutoInterface) updateLinkLocalAddresses() {
	ai.Mutex.RLock()
	names := make([]string, 0, len(ai.adoptedInterfaces))
	for name := range ai.adoptedInterfaces {
		names = append(names, name)
	}
	ai.Mutex.RUnlock()

	for _, ifname := range names {
		current := ai.currentLinkLocalAddr(ifname)
		if current == "" {
			continue
		}
		ai.Mutex.RLock()
		adopted, ok := ai.adoptedInterfaces[ifname]
		ai.Mutex.RUnlock()
		if !ok || adopted.linkLocalAddr == current {
			continue
		}
		ai.replaceDataListener(ifname, current)
	}
}

func (ai *AutoInterface) replaceDataListener(ifname, newAddr string) {
	ai.Mutex.Lock()
	adopted, ok := ai.adoptedInterfaces[ifname]
	if !ok {
		ai.Mutex.Unlock()
		return
	}
	oldAddr := adopted.linkLocalAddr
	adopted.linkLocalAddr = newAddr
	if conn, exists := ai.interfaceServers[ifname]; exists {
		go func(c *net.UDPConn) {
			_ = c.Close()
		}(conn)
		delete(ai.interfaceServers, ifname)
	}
	if outbound, exists := ai.outboundConns[ifname]; exists {
		_ = outbound.Close()
		delete(ai.outboundConns, ifname)
	}
	ai.Mutex.Unlock()

	debug.Log(debug.DebugInfo, "Replacing auto interface link-local address",
		"interface", ifname, "old", oldAddr, "new", newAddr)

	iface, err := net.InterfaceByName(ifname)
	if err != nil {
		debug.Log(debug.DebugError, "Failed to resolve interface for roam", "interface", ifname, "error", err)
		return
	}

	var started bool
	for attempt := 0; attempt < 8 && !started; attempt++ {
		time.Sleep(dataListenerRetryDelay)
		if err := ai.startDataListener(iface); err != nil {
			debug.Log(debug.DebugError, "Failed to restart data listener after roam",
				"interface", ifname, "attempt", attempt+1, "error", err)
			continue
		}
		outboundAddr := &net.UDPAddr{
			IP:   net.ParseIP(newAddr),
			Port: 0,
			Zone: ifname,
		}
		outboundConn, err := net.ListenUDP("udp6", outboundAddr)
		if err != nil {
			debug.Log(debug.DebugError, "Failed to recreate outbound socket after roam",
				"interface", ifname, "error", err)
			continue
		}
		ai.Mutex.Lock()
		ai.outboundConns[ifname] = outboundConn
		ai.Mutex.Unlock()
		started = true
	}
	if !started {
		debug.Log(debug.DebugError, "Could not restart auto interface data listener after roam", "interface", ifname)
	}
}

func (ai *AutoInterface) peerJobsUpdateLinkLocal() {
	if !ai.IsOnline() {
		return
	}
	ai.updateLinkLocalAddresses()
}
