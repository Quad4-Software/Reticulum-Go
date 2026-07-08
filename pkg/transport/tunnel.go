// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package transport

import (
	"crypto/sha256"
	"time"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/cryptography"
	"quad4/reticulum-go/pkg/debug"
	"quad4/reticulum-go/pkg/destination"
	"quad4/reticulum-go/pkg/identity"
	"quad4/reticulum-go/pkg/interfaces"
	"quad4/reticulum-go/pkg/packet"
)

const tunnelTimeout = 8 * 60 * 60 // 8 hours, matches Python Transport.TUNNEL_TIMEOUT

// TunnelInterface is implemented by logical interfaces that participate in
// Reticulum tunnel establishment (I2P peers, i2p_tunneled TCP clients).
type TunnelInterface = interfaces.TunnelPeer

type tunnelPathEntry struct {
	receivedFrom []byte
	hops         uint8
	expires      time.Time
	packetHash   []byte
}

type tunnelEntry struct {
	id      [32]byte
	iface   TunnelInterface
	paths   map[[16]byte]tunnelPathEntry
	expires time.Time
}

// InitializeTunnelHandler registers the tunnel synthesize control destination.
func (t *Transport) InitializeTunnelHandler() error {
	if t.transportIdentity == nil {
		return nil
	}
	inDest, err := destination.New(nil, destination.In, destination.Plain, "rnstransport", t, "tunnel", "synthesize")
	if err != nil {
		return err
	}
	inDest.SetPacketCallback(func(data []byte, iface common.NetworkInterface) {
		t.handleTunnelSynthesize(data, iface)
	})
	t.RegisterDestination(inDest.GetHash(), inDest)
	t.tunnelSynthOutHash = tunnelSynthOutHash()
	return nil
}

func tunnelSynthOutHash() []byte {
	name := "rnstransport.tunnel.synthesize"
	nameHash := sha256.Sum256([]byte(name))
	final := sha256.Sum256(nameHash[:10])
	return final[:16]
}

// SynthesizeTunnel broadcasts a signed tunnel-establishment packet on iface.
func (t *Transport) SynthesizeTunnel(iface TunnelInterface) error {
	if iface == nil || !iface.WantsTunnel() {
		return nil
	}
	t.mutex.RLock()
	id := t.transportIdentity
	outHash := t.tunnelSynthOutHash
	t.mutex.RUnlock()
	if id == nil {
		return nil
	}
	if len(outHash) == 0 {
		if err := t.InitializeTunnelHandler(); err != nil {
			return err
		}
		t.mutex.RLock()
		outHash = t.tunnelSynthOutHash
		t.mutex.RUnlock()
	}

	ifHash := iface.InterfaceHash()
	if len(ifHash) != identity.HashLength/8 {
		return nil
	}
	pubKey := id.GetPublicKey()
	tunnelIDData := append(append([]byte(nil), pubKey...), ifHash...)
	tunnelID := cryptography.Hash(tunnelIDData)
	randomHash := identity.GetRandomHash()
	signedData := append(append([]byte(nil), tunnelIDData...), randomHash...)
	sig, err := id.Sign(signedData)
	if err != nil {
		return err
	}
	payload := append(append([]byte(nil), signedData...), sig...)

	pkt := packet.NewPacket(
		packet.DestinationPlain,
		payload,
		packet.PacketTypeData,
		0x00,
		packet.PropagationBroadcast,
		packet.HeaderType1,
		nil,
		false,
		0x00,
	)
	pkt.DestinationHash = outHash
	if err := pkt.Pack(); err != nil {
		return err
	}
	if err := iface.Send(pkt.Raw, ""); err != nil {
		return err
	}
	iface.SetWantsTunnel(false)
	_ = tunnelID
	return nil
}

func (t *Transport) handleTunnelSynthesize(data []byte, iface common.NetworkInterface) {
	const expected = identity.KeySize/8 + identity.HashLength/8 + identity.TruncatedHashLength/8 + identity.SigLength/8
	if len(data) != expected {
		return
	}
	pubKey := data[:identity.KeySize/8]
	ifHash := data[identity.KeySize/8 : identity.KeySize/8+identity.HashLength/8]
	randomHash := data[identity.KeySize/8+identity.HashLength/8 : identity.KeySize/8+identity.HashLength/8+identity.TruncatedHashLength/8]
	sig := data[identity.KeySize/8+identity.HashLength/8+identity.TruncatedHashLength/8:]
	tunnelIDData := append(append([]byte(nil), pubKey...), ifHash...)
	tunnelID := cryptography.Hash(tunnelIDData)
	signedData := append(append([]byte(nil), tunnelIDData...), randomHash...)

	remote := identity.FromPublicKey(pubKey)
	if remote == nil || !remote.Verify(signedData, sig) {
		return
	}
	tunIface, ok := iface.(TunnelInterface)
	if !ok {
		return
	}
	t.handleTunnel(tunnelID, tunIface)
}

func (t *Transport) handleTunnel(tunnelID []byte, iface TunnelInterface) {
	if len(tunnelID) != 32 || iface == nil {
		return
	}
	var idKey [32]byte
	copy(idKey[:], tunnelID)
	expires := time.Now().Add(tunnelTimeout)

	t.tunnelMu.Lock()
	defer t.tunnelMu.Unlock()
	if t.tunnels == nil {
		t.tunnels = make(map[[32]byte]*tunnelEntry)
	}
	entry, exists := t.tunnels[idKey]
	if !exists {
		entry = &tunnelEntry{
			id:      idKey,
			iface:   iface,
			paths:   make(map[[16]byte]tunnelPathEntry),
			expires: expires,
		}
		t.tunnels[idKey] = entry
		iface.SetTunnelID(tunnelID)
		debug.Log(debug.DebugInfo, "Tunnel endpoint established", "tunnel", fmtHex(tunnelID[:8]))
		return
	}
	entry.iface = iface
	entry.expires = expires
	iface.SetTunnelID(tunnelID)
	debug.Log(debug.DebugInfo, "Tunnel endpoint reappeared, restoring paths", "tunnel", fmtHex(tunnelID[:8]))
	for destKey, pe := range entry.paths {
		if time.Now().After(pe.expires) {
			delete(entry.paths, destKey)
			continue
		}
		dest := destKey[:]
		nextHop := pe.receivedFrom
		if len(nextHop) == 0 {
			nextHop = dest
		}
		t.mutex.Lock()
		t.updatePathUnlocked(dest, nextHop, iface.GetName(), pe.hops, nil, pe.packetHash, time.Now())
		t.mutex.Unlock()
	}
}

// VoidTunnel clears the live interface reference for a tunnel table entry.
func (t *Transport) VoidTunnel(iface TunnelInterface) {
	if iface == nil {
		return
	}
	tid := iface.TunnelID()
	if len(tid) != 32 {
		return
	}
	var idKey [32]byte
	copy(idKey[:], tid)
	t.tunnelMu.Lock()
	defer t.tunnelMu.Unlock()
	if entry, ok := t.tunnels[idKey]; ok {
		entry.iface = nil
	}
}

func (t *Transport) associateTunnelPath(iface TunnelInterface, destHash, receivedFrom, packetHash []byte, hops uint8) {
	tid := iface.TunnelID()
	if len(tid) != 32 {
		return
	}
	var idKey [32]byte
	copy(idKey[:], tid)
	var destKey [16]byte
	if len(destHash) >= 16 {
		copy(destKey[:], destHash[:16])
	} else {
		copy(destKey[:], destHash)
	}
	rf := append([]byte(nil), receivedFrom...)
	ph := append([]byte(nil), packetHash...)

	t.tunnelMu.Lock()
	defer t.tunnelMu.Unlock()
	entry, ok := t.tunnels[idKey]
	if !ok || entry == nil {
		return
	}
	entry.paths[destKey] = tunnelPathEntry{
		receivedFrom: rf,
		hops:         hops,
		expires:      time.Now().Add(tunnelTimeout),
		packetHash:   ph,
	}
	entry.expires = time.Now().Add(tunnelTimeout)
}

func (t *Transport) cleanupExpiredTunnels() {
	now := time.Now()
	t.tunnelMu.Lock()
	defer t.tunnelMu.Unlock()
	for id, entry := range t.tunnels {
		if entry == nil || now.After(entry.expires) {
			delete(t.tunnels, id)
		}
	}
}

func fmtHex(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	const hexdigits = "0123456789abcdef"
	out := make([]byte, len(b)*2)
	for i, v := range b {
		out[i*2] = hexdigits[v>>4]
		out[i*2+1] = hexdigits[v&0x0f]
	}
	return string(out)
}
