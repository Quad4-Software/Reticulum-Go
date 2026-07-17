// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package harness

import (
	"fmt"
	"net"
	"time"
)

// MeshPeer is a TCP mesh endpoint used for NomadNet-style preflight.
type MeshPeer struct {
	Name string
	Host string
	Port int
}

// DefaultMeshPeers returns the public mesh TCP peers used by NomadNet live tests.
func DefaultMeshPeers() []MeshPeer {
	return []MeshPeer{
		{Name: "Beleth", Host: "rns.beleth.net", Port: 4242},
		{Name: "MichMesh", Host: "rns.michmesh.net", Port: 7822},
		{Name: "Quortal", Host: "reticulum.qortal.link", Port: 4242},
		{Name: "StoppedCold", Host: "rns.stoppedcold.com", Port: 4242},
		{Name: "Sydney", Host: "sydney.reticulum.au", Port: 4242},
	}
}

// MeshPreflight dials peers and returns how many accepted a TCP connection.
func MeshPreflight(peers []MeshPeer, perPeer time.Duration) (online int, err error) {
	if len(peers) == 0 {
		peers = DefaultMeshPeers()
	}
	if perPeer <= 0 {
		perPeer = 5 * time.Second
	}
	for _, p := range peers {
		addr := net.JoinHostPort(p.Host, fmt.Sprintf("%d", p.Port))
		c, dialErr := net.DialTimeout("tcp", addr, perPeer)
		if dialErr != nil {
			continue
		}
		_ = c.Close()
		online++
	}
	if online == 0 {
		return 0, fmt.Errorf("no mesh TCP peers reachable")
	}
	return online, nil
}
