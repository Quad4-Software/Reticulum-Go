// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package transport

import (
	"crypto/sha256"
	"math"
	"time"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/common"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/cryptography"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/debug"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/destination"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/identity"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/interfaces"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/packet"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/stamps"

	"github.com/Quad4-Software/msgpack/v5/pkg/msgpack"
)

const tunnelTimeout = 8 * time.Hour

// tunnelReconnectWindow is how long a tunnel entry waits for its interface
// to reappear (Python TUNNEL_RECONNECT_WINDOW, RNS 1.6.1).
const tunnelReconnectWindow = 10 * time.Minute

// tunnelSynthMaxLen is Python Packet.MDU: MTU 500 - HEADER_MAXSIZE 34 -
// IFAC_MIN_SIZE 1.
const tunnelSynthMaxLen = 465

// Tunnel synthesis ext_payload field tags (RNS.Discovery wire constants,
// duplicated here because pkg/discovery imports this package).
const (
	tunnelFieldTransportImpl byte = 0xFD
	tunnelFieldTransportVers byte = 0xFC
	tunnelFieldTransport     byte = 0x01
	tunnelFieldDstCost       byte = 0xFB
	tunnelFieldPRCost        byte = 0xFA
)

// maxTunnels bounds live tunnel table rows on this node.
const maxTunnels = 256

// TunnelInterface is implemented by logical interfaces that participate in
// Reticulum tunnel establishment (I2P peers, i2p_tunneled TCP clients).
type TunnelInterface = interfaces.TunnelPeer

// tunnelPeerState is the RNS 1.6.1 tunnel synthesis/remote-transport state
// carried on BaseInterface.
type tunnelPeerState interface {
	TunnelsAllowed() bool
	RemoteTransportHash() []byte
	SetRemoteTransportHash([]byte)
	SynthesisEmitted() bool
	SetSynthesisEmitted(bool)
	SynthesisReceived() bool
	SetSynthesisReceived(bool)
	LocalTunnelID() []byte
	SetLocalTunnelID([]byte)
	SetRemoteTunnelMeta(impl, vers string, isTN bool, dstCost, prCost float64)
}

// earlySynthesizer sends a synthesis packet before the interface is online
// (Python early_synthesis on TCPClient/BackboneClient interfaces).
type earlySynthesizer interface {
	EarlySynthesis(raw []byte) error
}

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
	return t.synthesizeTunnel(iface, false)
}

// tunnelExtPayload packs the RNS 1.6.1 synthesis metadata dict:
// TRANSPORT_IMPL, TRANSPORT_VERS, TRANSPORT, DST_COST, PR_COST.
func (t *Transport) tunnelExtPayload(iface common.NetworkInterface) ([]byte, error) {
	settings := t.StampsSettings()
	dstCost := t.interfaceDestinationCost(iface, settings)
	prCost := t.interfacePRCost(iface, settings)
	ext := map[uint8]any{
		tunnelFieldTransportImpl: common.ImplementationName,
		tunnelFieldTransportVers: common.ImplementationVersion(),
		tunnelFieldTransport:     t.transportEnabled(),
		tunnelFieldDstCost:       dstCost,
		tunnelFieldPRCost:        prCost,
	}
	return msgpack.Marshal(ext)
}

func (t *Transport) synthesizeTunnel(iface TunnelInterface, reverse bool) error {
	if iface == nil {
		return nil
	}
	if !reverse && !iface.WantsTunnel() {
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
	randomHash, err := identity.GetRandomHash()
	if err != nil {
		return err
	}
	extPayload, err := t.tunnelExtPayload(iface)
	if err != nil {
		return err
	}
	signedData := append(append(append([]byte(nil), tunnelIDData...), randomHash...), extPayload...)
	sig, err := id.Sign(signedData)
	if err != nil {
		return err
	}
	payload := append(append(append([]byte(nil), tunnelIDData...), randomHash...), sig...)
	payload = append(payload, extPayload...)

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
	if st, ok := iface.(tunnelPeerState); ok {
		st.SetSynthesisEmitted(true)
	}
	if iface.IsOnline() {
		if err := sendOnInterface(iface, pkt.Raw, ""); err != nil {
			return err
		}
	} else if es, ok := iface.(earlySynthesizer); ok {
		if err := es.EarlySynthesis(pkt.Raw); err != nil {
			return err
		}
	} else {
		return nil
	}
	iface.SetWantsTunnel(false)
	if !reverse {
		if st, ok := iface.(tunnelPeerState); ok {
			st.SetLocalTunnelID(tunnelID)
		}
	}
	return nil
}

// tunnelViolation reports a tunnel-synthesis protocol violation on iface.
func tunnelViolation(iface common.NetworkInterface, description string) {
	ifaceProtocolViolation(iface)
	debug.Log(debug.DebugInfo, "Tunnel synthesis protocol violation",
		"iface", ifaceNameSafe(iface), "reason", description)
}

func ifaceNameSafe(iface common.NetworkInterface) string {
	if iface == nil {
		return ""
	}
	return iface.GetName()
}

// detachTunnelInterface tears down an interface after an invalid or
// blackholed tunnel synthesis (Python _detach_interface internal_forced).
func (t *Transport) detachTunnelInterface(iface common.NetworkInterface) {
	if iface == nil {
		return
	}
	iface.Detach()
}

func (t *Transport) handleTunnelSynthesize(data []byte, iface common.NetworkInterface) {
	st, stOK := iface.(tunnelPeerState)
	if !stOK {
		return
	}
	if !st.TunnelsAllowed() {
		return
	}
	if len(st.RemoteTransportHash()) > 0 {
		tunnelViolation(iface, "tunnel synthesis packet on already established tunnel")
		return
	}
	if st.SynthesisReceived() {
		tunnelViolation(iface, "additional synthesis packet received")
		return
	}
	st.SetSynthesisReceived(true)

	const requiredLen = identity.KeySize/8 + identity.HashLength/8 + identity.TruncatedHashLength/8 + identity.SigLength/8
	if len(data) > tunnelSynthMaxLen {
		tunnelViolation(iface, "excessive tunnel synthesis packet size")
		return
	}
	if len(data) < requiredLen {
		tunnelViolation(iface, "insufficient tunnel synthesis packet size")
		return
	}
	keyLen := identity.KeySize / 8
	hashLen := identity.HashLength / 8
	truncLen := identity.TruncatedHashLength / 8
	sigLen := identity.SigLength / 8

	pubKey := data[:keyLen]
	ifHash := data[keyLen : keyLen+hashLen]
	randomHash := data[keyLen+hashLen : keyLen+hashLen+truncLen]
	sig := data[keyLen+hashLen+truncLen : keyLen+hashLen+truncLen+sigLen]
	extPayload := data[requiredLen:]
	tunnelIDData := append(append([]byte(nil), pubKey...), ifHash...)
	tunnelID := cryptography.Hash(tunnelIDData)
	signedData := append(append(append([]byte(nil), tunnelIDData...), randomHash...), extPayload...)

	remote := identity.FromPublicKey(pubKey)
	if remote == nil || !remote.Verify(signedData, sig) {
		debug.Log(debug.DebugInfo, "Invalid tunnel synthesis signature, tearing down interface",
			"iface", ifaceNameSafe(iface))
		t.detachTunnelInterface(iface)
		return
	}
	if tab := t.BlackholeTable(); tab != nil && tab.Has(remote.Hash()) {
		debug.Log(debug.DebugInfo, "Tunnel synthesis from blackholed transport node, tearing down interface",
			"iface", ifaceNameSafe(iface), "remote", fmtHex(remote.Hash()[:8]))
		t.detachTunnelInterface(iface)
		return
	}

	tunIface, ok := iface.(TunnelInterface)
	if !ok {
		return
	}

	if len(extPayload) > 0 {
		var ext map[any]any
		if err := msgpack.Unmarshal(extPayload, &ext); err != nil {
			tunnelViolation(iface, "undecodable ext_payload in tunnel synthesis packet")
			return
		}
		if ext == nil {
			tunnelViolation(iface, "invalid ext_payload data type in tunnel synthesis packet")
			return
		}
		get := func(key uint8) (any, bool) {
			for k, v := range ext {
				var n int64
				switch kk := k.(type) {
				case int8:
					n = int64(kk)
				case int16:
					n = int64(kk)
				case int32:
					n = int64(kk)
				case int64:
					n = kk
				case int:
					n = int64(kk)
				case uint8:
					n = int64(kk)
				case uint16:
					n = int64(kk)
				case uint32:
					n = int64(kk)
				case uint64:
					if kk > math.MaxInt64 {
						continue
					}
					n = int64(kk)
				case uint:
					if uint64(kk) > math.MaxInt64 {
						continue
					}
					n = int64(kk)
				default:
					continue
				}
				if n == int64(key) {
					return v, true
				}
			}
			return nil, false
		}
		implRaw, okImpl := get(tunnelFieldTransportImpl)
		versRaw, okVers := get(tunnelFieldTransportVers)
		tnRaw, okTN := get(tunnelFieldTransport)
		dstRaw, okDst := get(tunnelFieldDstCost)
		prRaw, okPR := get(tunnelFieldPRCost)
		switch {
		case !okImpl:
			tunnelViolation(iface, "missing implementation info in tunnel synthesis packet")
			return
		case !okVers:
			tunnelViolation(iface, "missing version info in tunnel synthesis packet")
			return
		case !okTN:
			tunnelViolation(iface, "missing transport status in tunnel synthesis packet")
			return
		case !okDst:
			tunnelViolation(iface, "missing destination cost in tunnel synthesis packet")
			return
		case !okPR:
			tunnelViolation(iface, "missing path request cost in tunnel synthesis packet")
			return
		}
		implName, ok := implRaw.(string)
		if !ok {
			tunnelViolation(iface, "invalid implementation info in tunnel synthesis packet")
			return
		}
		versStr, ok := versRaw.(string)
		if !ok || common.VersionTuple(versStr) == nil {
			tunnelViolation(iface, "invalid version info in tunnel synthesis packet")
			return
		}
		remoteImpl := common.SanitizeDiscoveryName(implName)
		if remoteImpl == "" || remoteImpl != implName {
			tunnelViolation(iface, "invalid implementation info in tunnel synthesis packet")
			return
		}
		dstCost, ok := extCostFloat(dstRaw)
		if !ok {
			tunnelViolation(iface, "invalid destination cost in tunnel synthesis packet")
			return
		}
		prCost, ok := extCostFloat(prRaw)
		if !ok {
			tunnelViolation(iface, "invalid path request cost in tunnel synthesis packet")
			return
		}
		isTN, ok := tnRaw.(bool)
		if !ok {
			tunnelViolation(iface, "invalid transport status in tunnel synthesis packet")
			return
		}
		st.SetRemoteTunnelMeta(remoteImpl, versStr, isTN,
			stamps.QuantizeCost(dstCost), stamps.QuantizeCost(prCost))
		if !st.SynthesisEmitted() {
			if err := t.synthesizeTunnel(tunIface, true); err != nil {
				debug.Log(debug.DebugError, "Reverse tunnel synthesis failed", "error", err)
			}
		}
	}

	st.SetRemoteTransportHash(remote.Hash())
	if len(st.LocalTunnelID()) == 0 {
		tunIface.SetTunnelID(tunnelID)
		t.handleTunnel(tunnelID, tunIface)
	}
}

// extCostFloat accepts only float-typed ext_payload costs, mirroring the
// strict Python `type(...) == float` check.
func extCostFloat(v any) (float64, bool) {
	f, ok := v.(float64)
	return f, ok
}

func (t *Transport) handleTunnel(tunnelID []byte, iface TunnelInterface) {
	if len(tunnelID) != 32 || iface == nil {
		return
	}
	var idKey [32]byte
	copy(idKey[:], tunnelID)
	now := time.Now()
	expires := now.Add(tunnelTimeout)

	t.tunnelMu.Lock()
	defer t.tunnelMu.Unlock()
	if t.tunnels == nil {
		t.tunnels = make(map[[32]byte]*tunnelEntry)
	}
	t.dropExpiredTunnels(now)
	entry, exists := t.tunnels[idKey]
	if !exists {
		if len(t.tunnels) >= maxTunnels {
			debug.Log(debug.DebugInfo, "Tunnel table at limit",
				"tunnel", fmtHex(tunnelID[:8]), "held", len(t.tunnels))
			return
		}
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
	t.dropExpiredTunnels(now)
}

func (t *Transport) dropExpiredTunnels(now time.Time) {
	t.mutex.RLock()
	defer t.mutex.RUnlock()
	for id, entry := range t.tunnels {
		if entry == nil {
			delete(t.tunnels, id)
			continue
		}
		if entry.expires.After(now.Add(2*tunnelTimeout)) || now.After(entry.expires) {
			delete(t.tunnels, id)
			continue
		}
		// RNS 1.6.1: a tunnel whose interface disappeared waits out the
		// reconnect window instead of expiring immediately.
		if entry.iface != nil {
			registered := t.interfaces[entry.iface.GetName()] == entry.iface
			if !registered {
				entry.iface = nil
				entry.expires = now.Add(tunnelReconnectWindow)
			}
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
