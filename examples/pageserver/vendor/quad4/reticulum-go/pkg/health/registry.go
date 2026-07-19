// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

// Package health provides node-local mesh integrity and link health counters.
// Counters stay on this node. They are for operator observability only.
package health

import (
	"sync"
	"time"
)

// Registry holds transport-wide and per-interface health counters.
type Registry struct {
	mu        sync.RWMutex
	transport [kindCount]windowedCounter
	byIface   map[string]*[kindCount]windowedCounter
}

// Default is the process-wide registry used by drop-site instrumentation.
var Default = NewRegistry()

// NewRegistry returns an empty Registry.
func NewRegistry() *Registry {
	return &Registry{
		byIface: make(map[string]*[kindCount]windowedCounter),
	}
}

// Inc increments kind on the transport totals and optionally on iface.
// An empty iface name updates transport totals only.
// Hot path: no heap alloc when the iface slot already exists.
func (r *Registry) Inc(iface string, kind Kind) {
	if r == nil || kind >= kindCount {
		return
	}
	now := time.Now().Unix()
	r.transport[kind].add(1, now)
	if iface == "" {
		return
	}
	slot := r.ifaceSlot(iface)
	slot[kind].add(1, now)
}

// ifaceSlot returns the per-iface counter array, creating it once if needed.
func (r *Registry) ifaceSlot(iface string) *[kindCount]windowedCounter {
	r.mu.RLock()
	slot := r.byIface[iface]
	r.mu.RUnlock()
	if slot != nil {
		return slot
	}
	r.mu.Lock()
	slot = r.byIface[iface]
	if slot == nil {
		slot = new([kindCount]windowedCounter)
		r.byIface[iface] = slot
	}
	r.mu.Unlock()
	return slot
}

// Inc is a convenience wrapper around Default.Inc.
func Inc(iface string, kind Kind) {
	Default.Inc(iface, kind)
}

// KindTotals is the lifetime and windowed counts for one kind.
type KindTotals struct {
	Total   uint64 `json:"total" msgpack:"total"`
	Rate60  uint64 `json:"rate_60s" msgpack:"rate_60s"`
	Rate300 uint64 `json:"rate_300s" msgpack:"rate_300s"`
}

// Snapshot is a point-in-time view of counters for one scope.
type Snapshot struct {
	IFACFail              KindTotals `json:"ifac_fail" msgpack:"ifac_fail"`
	HMACFail              KindTotals `json:"hmac_fail" msgpack:"hmac_fail"`
	UnpackFail            KindTotals `json:"unpack_fail" msgpack:"unpack_fail"`
	PaddingFail           KindTotals `json:"padding_fail" msgpack:"padding_fail"`
	AnnounceSigFail       KindTotals `json:"announce_sig_fail" msgpack:"announce_sig_fail"`
	ProofFail             KindTotals `json:"proof_fail" msgpack:"proof_fail"`
	LRProofHopMismatch    KindTotals `json:"lrproof_hop_mismatch" msgpack:"lrproof_hop_mismatch"`
	RequestSkewReject     KindTotals `json:"request_skew_reject" msgpack:"request_skew_reject"`
	BlackholeHit          KindTotals `json:"blackhole_hit" msgpack:"blackhole_hit"`
	LinkStaleClose        KindTotals `json:"link_stale_close" msgpack:"link_stale_close"`
	KeepaliveTimeout      KindTotals `json:"keepalive_timeout" msgpack:"keepalive_timeout"`
	ResourceStall         KindTotals `json:"resource_stall" msgpack:"resource_stall"`
	ResourceReq           KindTotals `json:"resource_req" msgpack:"resource_req"`
	ResourceHMU           KindTotals `json:"resource_hmu" msgpack:"resource_hmu"`
	ResourceComplete      KindTotals `json:"resource_complete" msgpack:"resource_complete"`
	NetmonFlap            KindTotals `json:"netmon_flap" msgpack:"netmon_flap"`
	RxOK                  KindTotals `json:"rx_ok" msgpack:"rx_ok"`
	AnnounceOK            KindTotals `json:"announce_ok" msgpack:"announce_ok"`
	AnnounceDup           KindTotals `json:"announce_dup" msgpack:"announce_dup"`
	PathRespSuppressed    KindTotals `json:"path_resp_suppressed" msgpack:"path_resp_suppressed"`
	PathReqDup            KindTotals `json:"path_req_dup" msgpack:"path_req_dup"`
	PathReqNoCache        KindTotals `json:"path_req_no_cache" msgpack:"path_req_no_cache"`
	PathRespQueuedSkip    KindTotals `json:"path_resp_queued_skip" msgpack:"path_resp_queued_skip"`
	LinkRelayUnknownIface KindTotals `json:"link_relay_unknown_iface" msgpack:"link_relay_unknown_iface"`
	// IntegrityFailRate is fails/(fails+ok) over the 60s window when sample size allows.
	IntegrityFailRate float64 `json:"integrity_fail_rate" msgpack:"integrity_fail_rate"`
	StaleCloses       uint64  `json:"stale_closes" msgpack:"stale_closes"`
}

// SnapshotTransport returns transport-wide counters.
func (r *Registry) SnapshotTransport() Snapshot {
	if r == nil {
		return Snapshot{}
	}
	return r.snapshotArray(&r.transport)
}

// SnapshotIface returns counters for iface, or an empty snapshot if unknown.
func (r *Registry) SnapshotIface(iface string) Snapshot {
	if r == nil || iface == "" {
		return Snapshot{}
	}
	r.mu.RLock()
	slot := r.byIface[iface]
	r.mu.RUnlock()
	if slot == nil {
		return Snapshot{}
	}
	return r.snapshotArray(slot)
}

func (r *Registry) snapshotArray(arr *[kindCount]windowedCounter) Snapshot {
	now := time.Now().Unix()
	s := Snapshot{
		IFACFail:              snapKind(arr, KindIFACFail, now),
		HMACFail:              snapKind(arr, KindHMACFail, now),
		UnpackFail:            snapKind(arr, KindUnpackFail, now),
		PaddingFail:           snapKind(arr, KindPaddingFail, now),
		AnnounceSigFail:       snapKind(arr, KindAnnounceSigFail, now),
		ProofFail:             snapKind(arr, KindProofFail, now),
		LRProofHopMismatch:    snapKind(arr, KindLRProofHopMismatch, now),
		RequestSkewReject:     snapKind(arr, KindRequestSkewReject, now),
		BlackholeHit:          snapKind(arr, KindBlackholeHit, now),
		LinkStaleClose:        snapKind(arr, KindLinkStaleClose, now),
		KeepaliveTimeout:      snapKind(arr, KindKeepaliveTimeout, now),
		ResourceStall:         snapKind(arr, KindResourceStall, now),
		ResourceReq:           snapKind(arr, KindResourceReq, now),
		ResourceHMU:           snapKind(arr, KindResourceHMU, now),
		ResourceComplete:      snapKind(arr, KindResourceComplete, now),
		NetmonFlap:            snapKind(arr, KindNetmonFlap, now),
		RxOK:                  snapKind(arr, KindRxOK, now),
		AnnounceOK:            snapKind(arr, KindAnnounceOK, now),
		AnnounceDup:           snapKind(arr, KindAnnounceDup, now),
		PathRespSuppressed:    snapKind(arr, KindPathRespSuppressed, now),
		PathReqDup:            snapKind(arr, KindPathReqDup, now),
		PathReqNoCache:        snapKind(arr, KindPathReqNoCache, now),
		PathRespQueuedSkip:    snapKind(arr, KindPathRespQueuedSkip, now),
		LinkRelayUnknownIface: snapKind(arr, KindLinkRelayUnknownIface, now),
	}
	s.StaleCloses = s.LinkStaleClose.Total
	fails := s.IFACFail.Rate60 + s.HMACFail.Rate60 + s.UnpackFail.Rate60 + s.PaddingFail.Rate60
	ok := s.RxOK.Rate60
	den := fails + ok
	if den > 0 {
		s.IntegrityFailRate = float64(fails) / float64(den)
	}
	return s
}

func snapKind(arr *[kindCount]windowedCounter, k Kind, unixSec int64) KindTotals {
	total, r60, r300 := arr[k].snapshot(unixSec)
	return KindTotals{Total: total, Rate60: r60, Rate300: r300}
}

// Reset clears all counters. Intended for tests.
func (r *Registry) Reset() {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.transport = [kindCount]windowedCounter{}
	r.byIface = make(map[string]*[kindCount]windowedCounter)
}

// IfaceCount returns how many interface slots are tracked. For tests and benches.
func (r *Registry) IfaceCount() int {
	if r == nil {
		return 0
	}
	r.mu.RLock()
	n := len(r.byIface)
	r.mu.RUnlock()
	return n
}
