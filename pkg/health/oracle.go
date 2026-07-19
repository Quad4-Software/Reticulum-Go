// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package health

// OracleSnapshot is a compact view used as a test oracle for health counters.
type OracleSnapshot struct {
	RxOK            uint64
	AnnounceOK      uint64
	AnnounceDup     uint64
	UnpackFail      uint64
	HMACFail        uint64
	IFACFail        uint64
	AnnounceSigFail uint64
	PathReqDup      uint64
}

// TransportOracle returns lifetime totals suitable for delta assertions in tests.
func (r *Registry) TransportOracle() OracleSnapshot {
	if r == nil {
		return OracleSnapshot{}
	}
	s := r.SnapshotTransport()
	return OracleSnapshot{
		RxOK:            s.RxOK.Total,
		AnnounceOK:      s.AnnounceOK.Total,
		AnnounceDup:     s.AnnounceDup.Total,
		UnpackFail:      s.UnpackFail.Total,
		HMACFail:        s.HMACFail.Total,
		IFACFail:        s.IFACFail.Total,
		AnnounceSigFail: s.AnnounceSigFail.Total,
		PathReqDup:      s.PathReqDup.Total,
	}
}

// Delta returns after - before for each counter (saturating at zero).
func (before OracleSnapshot) Delta(after OracleSnapshot) OracleSnapshot {
	sub := func(a, b uint64) uint64 {
		if a >= b {
			return a - b
		}
		return 0
	}
	return OracleSnapshot{
		RxOK:            sub(after.RxOK, before.RxOK),
		AnnounceOK:      sub(after.AnnounceOK, before.AnnounceOK),
		AnnounceDup:     sub(after.AnnounceDup, before.AnnounceDup),
		UnpackFail:      sub(after.UnpackFail, before.UnpackFail),
		HMACFail:        sub(after.HMACFail, before.HMACFail),
		IFACFail:        sub(after.IFACFail, before.IFACFail),
		AnnounceSigFail: sub(after.AnnounceSigFail, before.AnnounceSigFail),
		PathReqDup:      sub(after.PathReqDup, before.PathReqDup),
	}
}

// IntegrityFails is the sum of common integrity-failure counters in the delta.
func (d OracleSnapshot) IntegrityFails() uint64 {
	return d.UnpackFail + d.HMACFail + d.IFACFail + d.AnnounceSigFail
}
