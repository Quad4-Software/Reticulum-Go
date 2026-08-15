// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package interfaces

import (
	"fmt"
	"net"
	"time"

	"quad4/reticulum-go/pkg/debug"
)

const autoRescanInterval = 10 * time.Second

// SetWatchInterfaces enables periodic NIC rescan from peerJobs (watch_interfaces).
func (ai *AutoInterface) SetWatchInterfaces(enabled bool) {
	ai.Mutex.Lock()
	ai.watchInterfaces = enabled
	ai.Mutex.Unlock()
}

// RescanInterfaces adds new NICs and removes disappeared ones.
func (ai *AutoInterface) RescanInterfaces() error {
	if !ai.IsOnline() {
		return fmt.Errorf("auto interface offline")
	}
	ifaces, err := net.Interfaces()
	if err != nil {
		return err
	}
	present := make(map[string]*net.Interface)
	for i := range ifaces {
		iface := &ifaces[i]
		if ai.shouldIgnoreInterface(iface.Name) {
			continue
		}
		if len(ai.allowedInterfaces) > 0 && !ai.isAllowedInterface(iface.Name) {
			continue
		}
		if iface.Flags&net.FlagUp == 0 {
			continue
		}
		present[iface.Name] = iface
	}

	ai.Mutex.Lock()
	adopted := make([]string, 0, len(ai.adoptedInterfaces))
	for name := range ai.adoptedInterfaces {
		adopted = append(adopted, name)
	}
	ai.Mutex.Unlock()

	for _, name := range adopted {
		if _, ok := present[name]; !ok {
			ai.removeInterface(name)
		}
	}

	for name, iface := range present {
		ai.Mutex.RLock()
		_, exists := ai.adoptedInterfaces[name]
		ai.Mutex.RUnlock()
		if exists {
			continue
		}
		ifaceCopy := *iface
		if err := ai.configureInterface(&ifaceCopy); err != nil {
			debug.Log(debug.DebugVerbose, "Rescan: failed to configure interface", "name", name, "error", err)
		}
	}
	ai.Mutex.Lock()
	ai.lastRescan = time.Now()
	ai.Mutex.Unlock()
	return nil
}

func (ai *AutoInterface) removeInterface(name string) {
	ai.Mutex.Lock()
	delete(ai.adoptedInterfaces, name)
	delete(ai.multicastEchoes, name)
	delete(ai.timedOutInterfaces, name)
	if conn, ok := ai.discoveryServers[name]; ok {
		_ = conn.Close()
		delete(ai.discoveryServers, name)
	}
	if conn, ok := ai.unicastDiscoveryServers[name]; ok {
		_ = conn.Close()
		delete(ai.unicastDiscoveryServers, name)
	}
	if conn, ok := ai.interfaceServers[name]; ok {
		_ = conn.Close()
		delete(ai.interfaceServers, name)
	}
	if conn, ok := ai.outboundConns[name]; ok {
		_ = conn.Close()
		delete(ai.outboundConns, name)
	}
	for key, peer := range ai.peers {
		if peer.ifaceName == name {
			delete(ai.peers, key)
		}
	}
	ai.Mutex.Unlock()
	debug.Log(debug.DebugInfo, "Removed auto interface", "interface", name)
}

func (ai *AutoInterface) maybeRescanLocked(now time.Time) {
	if !ai.watchInterfaces {
		return
	}
	if !ai.lastRescan.IsZero() && now.Sub(ai.lastRescan) < autoRescanInterval {
		return
	}
	ai.Mutex.Unlock()
	_ = ai.RescanInterfaces()
	ai.Mutex.Lock()
}
