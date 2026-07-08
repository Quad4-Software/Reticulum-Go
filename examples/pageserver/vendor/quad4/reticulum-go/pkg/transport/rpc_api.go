// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package transport

import (
	"quad4/reticulum-go/pkg/cryptography"
	"quad4/reticulum-go/pkg/packet"
)

// PathTableEntry mirrors Python get_path_table() rows for RPC interop.
type PathTableEntry struct {
	Hash      []byte  `msgpack:"hash"`
	Timestamp float64 `msgpack:"timestamp"`
	Via       []byte  `msgpack:"via"`
	Hops      uint8   `msgpack:"hops"`
	Expires   float64 `msgpack:"expires"`
	Interface string  `msgpack:"interface"`
}

// InterfaceStat mirrors the subset of Python get_interface_stats() used by tools.
type InterfaceStat struct {
	Name           string  `msgpack:"name"`
	ShortName      string  `msgpack:"short_name"`
	Hash           []byte  `msgpack:"hash"`
	Type           string  `msgpack:"type"`
	RXB            uint64  `msgpack:"rxb"`
	TXB            uint64  `msgpack:"txb"`
	Status         bool    `msgpack:"status"`
	Mode           byte    `msgpack:"mode"`
	Clients        *int    `msgpack:"clients"`
	Bitrate        int64   `msgpack:"bitrate"`
	I2PConnectable *bool   `msgpack:"i2p_connectable,omitempty"`
	I2PB32         *string `msgpack:"i2p_b32,omitempty"`
	TunnelState    *string `msgpack:"tunnelstate,omitempty"`
}

// InterfaceStatsResponse mirrors Python get_interface_stats() top-level map.
type InterfaceStatsResponse struct {
	Interfaces      []InterfaceStat `msgpack:"interfaces"`
	RXB             uint64          `msgpack:"rxb"`
	TXB             uint64          `msgpack:"txb"`
	RXS             float64         `msgpack:"rxs"`
	TXS             float64         `msgpack:"txs"`
	TransportID     []byte          `msgpack:"transport_id"`
	TransportUptime float64         `msgpack:"transport_uptime"`
}

// RateTableEntry mirrors Python get_rate_table() rows.
type RateTableEntry struct {
	Hash           []byte    `msgpack:"hash"`
	Last           float64   `msgpack:"last"`
	RateViolations int       `msgpack:"rate_violations"`
	BlockedUntil   float64   `msgpack:"blocked_until"`
	Timestamps     []float64 `msgpack:"timestamps"`
}

func (t *Transport) SetConnectedToSharedInstance(v bool) {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	if t.config != nil {
		t.config.ConnectedToSharedInstance = v
	}
}

func (t *Transport) ConnectedToSharedInstance() bool {
	t.mutex.RLock()
	defer t.mutex.RUnlock()
	if t.config == nil {
		return false
	}
	return t.config.ConnectedToSharedInstance
}

func (t *Transport) TransportIdentityHash() []byte {
	t.mutex.RLock()
	defer t.mutex.RUnlock()
	if t.transportIdentity == nil {
		return nil
	}
	return t.transportIdentity.Hash()
}

func (t *Transport) RPCAuthKey() []byte {
	t.mutex.RLock()
	id := t.transportIdentity
	t.mutex.RUnlock()
	if id == nil {
		return nil
	}
	priv, err := id.GetPrivateKey()
	if err != nil {
		return nil
	}
	return cryptography.Hash(priv)
}

func (t *Transport) GetPathTable(maxHops *int) []PathTableEntry {
	ttl := float64(PathRequestTTL)
	t.mutex.RLock()
	defer t.mutex.RUnlock()
	out := make([]PathTableEntry, 0, len(t.paths))
	truncLen := packet.TruncatedHashLength / 8
	for key, path := range t.paths {
		if path == nil {
			continue
		}
		hops := path.HopCount
		if maxHops != nil && int(hops) > *maxHops {
			continue
		}
		hash := append([]byte(nil), key[:truncLen]...)
		entry := PathTableEntry{
			Hash:      hash,
			Timestamp: float64(path.LastUpdated.Unix()),
			Via:       append([]byte(nil), path.NextHop...),
			Hops:      hops,
			Expires:   float64(path.LastUpdated.Unix()) + ttl,
		}
		if path.Interface != nil {
			entry.Interface = path.Interface.GetName()
		}
		out = append(out, entry)
	}
	return out
}

func (t *Transport) GetInterfaceStatsRPC() InterfaceStatsResponse {
	t.mutex.RLock()
	defer t.mutex.RUnlock()
	resp := InterfaceStatsResponse{
		Interfaces: make([]InterfaceStat, 0, len(t.interfaces)),
	}
	for _, iface := range t.interfaces {
		if iface == nil {
			continue
		}
		st := InterfaceStat{
			Name:      iface.GetName(),
			ShortName: iface.GetName(),
			Type:      "Interface",
			Status:    iface.IsOnline(),
			Mode:      byte(iface.GetMode()),
			RXB:       iface.GetRxBytes(),
			TXB:       iface.GetTxBytes(),
		}
		if parent, ok := iface.(interface {
			Connectable() bool
			Base32() string
			Clients() int
		}); ok {
			connectable := parent.Connectable()
			st.I2PConnectable = &connectable
			if b32 := parent.Base32(); b32 != "" {
				endpoint := b32 + ".b32.i2p"
				st.I2PB32 = &endpoint
			}
			clients := parent.Clients()
			st.Clients = &clients
		}
		if peer, ok := iface.(interface{ TunnelState() uint32 }); ok {
			label := i2pTunnelStateLabel(peer.TunnelState())
			st.TunnelState = &label
		}
		resp.Interfaces = append(resp.Interfaces, st)
	}
	if t.transportIdentity != nil {
		resp.TransportID = t.transportIdentity.Hash()
	}
	return resp
}

func i2pTunnelStateLabel(state uint32) string {
	switch state {
	case 0x01:
		return "Tunnel Active"
	case 0x02:
		return "Tunnel Unresponsive"
	default:
		return "Creating Tunnel"
	}
}

func (t *Transport) GetRateTableRPC() []RateTableEntry {
	return nil
}

func (t *Transport) DropPathRPC(destinationHash []byte) bool {
	t.ExpirePath(destinationHash)
	return true
}

func (t *Transport) DropAllViaRPC(transportHash []byte) int {
	dropped := 0
	t.mutex.Lock()
	defer t.mutex.Unlock()
	for key, path := range t.paths {
		if path != nil && len(path.NextHop) == len(transportHash) {
			if string(path.NextHop) == string(transportHash) {
				delete(t.paths, key)
				dropped++
			}
		}
	}
	return dropped
}

func (t *Transport) DropAnnounceQueuesRPC() int {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	n := len(t.heldAnnounces)
	t.heldAnnounces = make(map[string]*PathAnnounceEntry)
	return n
}

func (t *Transport) GetLinkCountRPC() int {
	t.mutex.RLock()
	defer t.mutex.RUnlock()
	return len(t.links)
}

func (t *Transport) GetNextHopRPC(destinationHash []byte) []byte {
	return t.NextHop(destinationHash)
}

func (t *Transport) GetNextHopIfNameRPC(destinationHash []byte) string {
	return t.NextHopInterface(destinationHash)
}

func (t *Transport) GetFirstHopTimeoutRPC(destinationHash []byte) float64 {
	hops := t.HopsTo(destinationHash)
	if hops >= PathfinderM {
		return float64(EstablishmentTimeoutPerHop)
	}
	return float64(EstablishmentTimeoutPerHop) * float64(max(1, int(hops)))
}

func (t *Transport) IsBlackholedRPC(identityHash []byte) bool {
	tab := t.BlackholeTable()
	if tab == nil {
		return false
	}
	return tab.Has(identityHash)
}

func (t *Transport) GetBlackholedIdentitiesRPC() []map[string]any {
	tab := t.BlackholeTable()
	if tab == nil {
		return nil
	}
	snap := tab.Snapshot()
	out := make([]map[string]any, 0, len(snap))
	for _, e := range snap {
		out = append(out, map[string]any{
			"identity": e.Hash[:],
			"until":    e.Entry.Until,
			"reason":   e.Entry.Reason,
		})
	}
	return out
}
