// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package transport

import (
	"time"

	"quad4/reticulum-go/pkg/cryptography"
	"quad4/reticulum-go/pkg/health"
	"quad4/reticulum-go/pkg/packet"
)

// PathTableEntry is one path-table row for shared-instance RPC.
type PathTableEntry struct {
	Hash      []byte  `msgpack:"hash"`
	Timestamp float64 `msgpack:"timestamp"`
	Via       []byte  `msgpack:"via"`
	Hops      uint8   `msgpack:"hops"`
	Expires   float64 `msgpack:"expires"`
	Interface string  `msgpack:"interface"`
}

// InterfaceStat is the per-interface stats subset used by status tools.
type InterfaceStat struct {
	Name                      string   `msgpack:"name"`
	ShortName                 string   `msgpack:"short_name"`
	Hash                      []byte   `msgpack:"hash"`
	Type                      string   `msgpack:"type"`
	RXB                       uint64   `msgpack:"rxb"`
	TXB                       uint64   `msgpack:"txb"`
	RXS                       float64  `msgpack:"rxs"`
	TXS                       float64  `msgpack:"txs"`
	IncomingAnnounceFrequency float64  `msgpack:"incoming_announce_frequency"`
	OutgoingAnnounceFrequency float64  `msgpack:"outgoing_announce_frequency"`
	IncomingPRFrequency       float64  `msgpack:"incoming_pr_frequency"`
	OutgoingPRFrequency       float64  `msgpack:"outgoing_pr_frequency"`
	HeldAnnounces             int      `msgpack:"held_announces"`
	BurstActive               bool     `msgpack:"burst_active"`
	PRBurstActive             bool     `msgpack:"pr_burst_active"`
	Status                    bool     `msgpack:"status"`
	Mode                      byte     `msgpack:"mode"`
	Clients                   *int     `msgpack:"clients"`
	Bitrate                   int64    `msgpack:"bitrate"`
	RTTMs                     *float64 `msgpack:"rtt_ms,omitempty"`
	BandwidthAvailable        *bool    `msgpack:"bandwidth_available,omitempty"`
	I2PConnectable            *bool    `msgpack:"i2p_connectable,omitempty"`
	I2PB32                    *string  `msgpack:"i2p_b32,omitempty"`
	TunnelState               *string  `msgpack:"tunnelstate,omitempty"`
	I2PLastError              *string  `msgpack:"i2p_last_error,omitempty"`
	IFACFail                  uint64   `msgpack:"ifac_fail"`
	HMACFail                  uint64   `msgpack:"hmac_fail"`
	AnnounceSigFail           uint64   `msgpack:"announce_sig_fail"`
	UnpackFail                uint64   `msgpack:"unpack_fail"`
	PaddingFail               uint64   `msgpack:"padding_fail"`
	ProofFail                 uint64   `msgpack:"proof_fail"`
	LRProofHopMismatch        uint64   `msgpack:"lrproof_hop_mismatch"`
	RequestSkewReject         uint64   `msgpack:"request_skew_reject"`
	BlackholeHit              uint64   `msgpack:"blackhole_hit"`
	LinkStaleClose            uint64   `msgpack:"link_stale_close"`
	KeepaliveTimeout          uint64   `msgpack:"keepalive_timeout"`
	ResourceStall             uint64   `msgpack:"resource_stall"`
	ResourceReq               uint64   `msgpack:"resource_req"`
	ResourceHMU               uint64   `msgpack:"resource_hmu"`
	ResourceComplete          uint64   `msgpack:"resource_complete"`
	RxOK                      uint64   `msgpack:"rx_ok"`
	AnnounceOK                uint64   `msgpack:"announce_ok"`
	AnnounceDup               uint64   `msgpack:"announce_dup"`
	PathRespSuppressed        uint64   `msgpack:"path_resp_suppressed"`
	PathReqDup                uint64   `msgpack:"path_req_dup"`
	PathReqNoCache            uint64   `msgpack:"path_req_no_cache"`
	PathRespQueuedSkip        uint64   `msgpack:"path_resp_queued_skip"`
	LinkRelayUnknownIface     uint64   `msgpack:"link_relay_unknown_iface"`
	IntegrityFailRate         float64  `msgpack:"integrity_fail_rate"`
	IntegritySamples60        uint64   `msgpack:"integrity_samples_60s"`
	StaleCloses               uint64   `msgpack:"stale_closes"`
	ActiveLinks               int      `msgpack:"active_links,omitempty"`
}

// InterfaceStatsResponse is the top-level interface stats RPC payload.
type InterfaceStatsResponse struct {
	Interfaces      []InterfaceStat `msgpack:"interfaces"`
	RXB             uint64          `msgpack:"rxb"`
	TXB             uint64          `msgpack:"txb"`
	RXS             float64         `msgpack:"rxs"`
	TXS             float64         `msgpack:"txs"`
	TransportID     []byte          `msgpack:"transport_id"`
	TransportUptime float64         `msgpack:"transport_uptime"`
	NetmonFlap      uint64          `msgpack:"netmon_flap"`
	ActiveLinks     int             `msgpack:"active_links"`
	Health          health.Snapshot `msgpack:"health"`
}

// RateTableEntry is one rate-table row for shared-instance RPC.
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
	id := t.rpcIdentity
	if id == nil {
		id = t.transportIdentity
	}
	t.mutex.RUnlock()
	if id == nil {
		return nil
	}
	priv, err := id.GetPrivateKey()
	if err != nil {
		return nil
	}
	sum := cryptography.Hash(priv)
	for i := range priv {
		priv[i] = 0
	}
	return sum
}

func (t *Transport) GetPathTable(maxHops *int) []PathTableEntry {
	t.mutex.RLock()
	defer t.mutex.RUnlock()
	now := time.Now()
	out := make([]PathTableEntry, 0, len(t.paths))
	truncLen := packet.TruncatedHashLength
	for key, path := range t.paths {
		if path == nil || pathExpired(path, now) {
			continue
		}
		hops := path.HopCount
		if maxHops != nil && int(hops) > *maxHops {
			continue
		}
		hash := append([]byte(nil), key[:truncLen]...)
		expires := float64(path.LastUpdated.Unix()) + float64(PathfinderE)
		if !path.Expires.IsZero() {
			expires = float64(path.Expires.Unix())
		}
		entry := PathTableEntry{
			Hash:      hash,
			Timestamp: float64(path.LastUpdated.Unix()),
			Via:       append([]byte(nil), path.NextHop...),
			Hops:      hops,
			Expires:   expires,
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
	var rxTotal, txTotal uint64
	var rxsTotal, txsTotal float64
	for _, iface := range t.interfaces {
		if iface == nil {
			continue
		}
		if sampler, ok := iface.(interface{ SampleTraffic() }); ok {
			sampler.SampleTraffic()
		}
		rx := iface.GetRxBytes()
		tx := iface.GetTxBytes()
		rxTotal += rx
		txTotal += tx
		st := InterfaceStat{
			Name:      iface.GetName(),
			ShortName: iface.GetName(),
			Type:      "Interface",
			Status:    iface.IsOnline(),
			Mode:      byte(iface.GetMode()),
			RXB:       rx,
			TXB:       tx,
		}
		if hasher, ok := iface.(interface{ InterfaceHash() []byte }); ok {
			st.Hash = hasher.InterfaceHash()
		}
		switch br := iface.(type) {
		case interface{ GetBitrate() int64 }:
			st.Bitrate = br.GetBitrate()
		case interface{ GetBitrate() int }:
			st.Bitrate = int64(br.GetBitrate())
		case interface{ GetBitrate() uint64 }:
			st.Bitrate = int64(br.GetBitrate()) // #nosec G115 -- bitrate display only
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
		if peer, ok := iface.(interface{ LastError() string }); ok {
			if errText := peer.LastError(); errText != "" {
				st.I2PLastError = &errText
			}
		}
		if v, ok := iface.(interface{ IncomingAnnounceFrequency() float64 }); ok {
			st.IncomingAnnounceFrequency = v.IncomingAnnounceFrequency()
		}
		if v, ok := iface.(interface{ OutgoingAnnounceFrequency() float64 }); ok {
			st.OutgoingAnnounceFrequency = v.OutgoingAnnounceFrequency()
		}
		if v, ok := iface.(interface{ IncomingPRFrequency() float64 }); ok {
			st.IncomingPRFrequency = v.IncomingPRFrequency()
		}
		if v, ok := iface.(interface{ OutgoingPRFrequency() float64 }); ok {
			st.OutgoingPRFrequency = v.OutgoingPRFrequency()
		}
		if v, ok := iface.(interface{ PRBurstActive() bool }); ok {
			st.PRBurstActive = v.PRBurstActive()
		}
		if v, ok := iface.(interface{ GetRxSpeed() float64 }); ok {
			st.RXS = v.GetRxSpeed()
			rxsTotal += st.RXS
		}
		if v, ok := iface.(interface{ GetTxSpeed() float64 }); ok {
			st.TXS = v.GetTxSpeed()
			txsTotal += st.TXS
		}
		if v, ok := iface.(interface{ GetRTT() time.Duration }); ok {
			if d := v.GetRTT(); d > 0 {
				ms := d.Seconds() * 1000
				st.RTTMs = &ms
			}
		}
		if v, ok := iface.(interface{ GetBandwidthAvailable() bool }); ok {
			avail := v.GetBandwidthAvailable()
			st.BandwidthAvailable = &avail
		}
		if t.ifaceStates != nil {
			if stt := t.ifaceStates.get(iface.GetName()); stt != nil && stt.ingress != nil {
				st.HeldAnnounces = stt.ingress.HeldCount()
				st.BurstActive = stt.ingress.InBurst()
			}
		}
		hs := health.Default.SnapshotIface(iface.GetName())
		st.IFACFail = hs.IFACFail.Total
		st.HMACFail = hs.HMACFail.Total
		st.AnnounceSigFail = hs.AnnounceSigFail.Total
		st.UnpackFail = hs.UnpackFail.Total
		st.PaddingFail = hs.PaddingFail.Total
		st.ProofFail = hs.ProofFail.Total
		st.LRProofHopMismatch = hs.LRProofHopMismatch.Total
		st.RequestSkewReject = hs.RequestSkewReject.Total
		st.BlackholeHit = hs.BlackholeHit.Total
		st.LinkStaleClose = hs.LinkStaleClose.Total
		st.KeepaliveTimeout = hs.KeepaliveTimeout.Total
		st.ResourceStall = hs.ResourceStall.Total
		st.ResourceReq = hs.ResourceReq.Total
		st.ResourceHMU = hs.ResourceHMU.Total
		st.ResourceComplete = hs.ResourceComplete.Total
		st.RxOK = hs.RxOK.Total
		st.AnnounceOK = hs.AnnounceOK.Total
		st.AnnounceDup = hs.AnnounceDup.Total
		st.PathRespSuppressed = hs.PathRespSuppressed.Total
		st.PathReqDup = hs.PathReqDup.Total
		st.PathReqNoCache = hs.PathReqNoCache.Total
		st.PathRespQueuedSkip = hs.PathRespQueuedSkip.Total
		st.LinkRelayUnknownIface = hs.LinkRelayUnknownIface.Total
		st.IntegrityFailRate = hs.IntegrityFailRate
		st.IntegritySamples60 = hs.IFACFail.Rate60 + hs.HMACFail.Rate60 + hs.UnpackFail.Rate60 + hs.PaddingFail.Rate60 + hs.RxOK.Rate60
		st.StaleCloses = hs.StaleCloses
		resp.Interfaces = append(resp.Interfaces, st)
	}
	resp.RXB = rxTotal
	resp.TXB = txTotal
	resp.RXS = rxsTotal
	resp.TXS = txsTotal
	trHealth := health.Default.SnapshotTransport()
	resp.Health = trHealth
	resp.NetmonFlap = trHealth.NetmonFlap.Total
	resp.ActiveLinks = t.countActiveLinksLocked()
	if t.transportIdentity != nil {
		resp.TransportID = t.transportIdentity.Hash()
	}
	if !t.startTime.IsZero() {
		resp.TransportUptime = time.Since(t.startTime).Seconds()
	}
	return resp
}

func (t *Transport) countActiveLinksLocked() int {
	n := 0
	for _, l := range t.links {
		if l == nil {
			continue
		}
		if statuser, ok := l.(interface{ GetStatus() byte }); ok {
			// Link.StatusActive is 0x02 in pkg/link.
			if statuser.GetStatus() == 0x02 {
				n++
			}
		}
	}
	return n
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
	t.mutex.RLock()
	defer t.mutex.RUnlock()
	if t.ifaceStates == nil {
		return nil
	}
	out := make([]RateTableEntry, 0)
	for _, e := range t.ifaceStates.snapshot() {
		if e.state == nil || e.state.ingress == nil {
			continue
		}
		out = append(out, RateTableEntry{
			Hash: []byte(e.name),
			Last: float64(time.Now().Unix()),
		})
	}
	return out
}

func (t *Transport) DropPathRPC(destinationHash []byte) bool {
	if t == nil || len(destinationHash) != 16 {
		return false
	}
	had := t.HasPath(destinationHash)
	t.ExpirePath(destinationHash)
	return had
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

func (t *Transport) GetBlackholedIdentitiesRPC() map[string]any {
	tab := t.BlackholeTable()
	if tab == nil {
		return map[string]any{}
	}
	snap := tab.Snapshot()
	out := make(map[string]any, len(snap))
	for _, e := range snap {
		// Blackhole maps are keyed by raw identity hash bytes. Msgpack map
		// keys that are binary decode as string in some clients, so use hex for
		// stable Go JSON tools and also include identity field in the value.
		key := string(e.Hash[:])
		out[key] = map[string]any{
			"until":  e.Entry.Until,
			"reason": e.Entry.Reason,
			"source": append([]byte(nil), e.Entry.Source...),
		}
	}
	return out
}
