// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package link

import "time"

func (l *Link) TrackPhyStats(track bool) {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	l.trackPhyStats = track
}
func (l *Link) UpdatePhyStats(rssi, snr, q float64) {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	if l.trackPhyStats {
		l.rssi = rssi
		l.snr = snr
		l.q = q
	}
}
func (l *Link) GetRSSI() float64 {
	l.mutex.RLock()
	defer l.mutex.RUnlock()
	if !l.trackPhyStats {
		return 0.0
	}
	return l.rssi
}
func (l *Link) GetSNR() float64 {
	l.mutex.RLock()
	defer l.mutex.RUnlock()
	if !l.trackPhyStats {
		return 0.0
	}
	return l.snr
}
func (l *Link) GetQ() float64 {
	l.mutex.RLock()
	defer l.mutex.RUnlock()
	if !l.trackPhyStats {
		return 0.0
	}
	return l.q
}
func (l *Link) GetEstablishmentRate() float64 {
	l.mutex.RLock()
	defer l.mutex.RUnlock()
	return l.establishmentRate
}
func (l *Link) GetAge() float64 {
	l.mutex.RLock()
	defer l.mutex.RUnlock()
	if l.establishedAt.IsZero() {
		return 0.0
	}
	return time.Since(l.establishedAt).Seconds()
}

// NoInboundFor returns the seconds elapsed since the last inbound packet.
func (l *Link) NoInboundFor() float64 {
	ns := l.lastInboundNs.Load()
	if ns == 0 {
		return 0.0
	}
	return time.Since(time.Unix(0, ns)).Seconds()
}

// NoOutboundFor returns the seconds elapsed since the last outbound packet.
func (l *Link) NoOutboundFor() float64 {
	ns := l.lastOutboundNs.Load()
	if ns == 0 {
		return 0.0
	}
	return time.Since(time.Unix(0, ns)).Seconds()
}

// NoDataFor returns the seconds since the most recent data packet (sent or received).
func (l *Link) NoDataFor() float64 {
	rxNs := l.lastDataReceivedNs.Load()
	txNs := l.lastDataSentNs.Load()
	last := max(txNs, rxNs)
	if last == 0 {
		return 0.0
	}
	return time.Since(time.Unix(0, last)).Seconds()
}

// InactiveFor returns the seconds since the most recent inbound or outbound packet.
func (l *Link) InactiveFor() float64 {
	inNs := l.lastInboundNs.Load()
	outNs := l.lastOutboundNs.Load()
	last := max(outNs, inNs)
	if last == 0 {
		return 0.0
	}
	return time.Since(time.Unix(0, last)).Seconds()
}

// nsToTime converts a UnixNano timestamp (0 means zero time) into a time.Time.
func nsToTime(ns int64) time.Time {
	if ns == 0 {
		return time.Time{}
	}
	return time.Unix(0, ns)
}
func (l *Link) recordOutbound() {
	l.lastOutboundNs.Store(time.Now().UnixNano())
}
func (l *Link) recordKeepaliveOutbound() {
	now := time.Now().UnixNano()
	l.lastOutboundNs.Store(now)
	l.lastKeepaliveNs.Store(now)
}
func (l *Link) recordOutboundData() {
	now := time.Now().UnixNano()
	l.lastOutboundNs.Store(now)
	l.lastDataSentNs.Store(now)
}
func (l *Link) recordInbound(isData bool) {
	now := time.Now().UnixNano()
	l.lastInboundNs.Store(now)
	if isData {
		l.lastDataReceivedNs.Store(now)
	}
}
func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
func (l *Link) GetRTT() float64 {
	l.mutex.RLock()
	defer l.mutex.RUnlock()
	return l.rtt
}
func (l *Link) RTT() float64 {
	return l.GetRTT()
}
func (l *Link) SetRTT(rtt float64) {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	l.rtt = rtt
}

// EstablishmentTimeout is the wait this link uses for proof and RTT during
// handshake. Initiators set it from first-hop airtime plus per-hop timeout.
// Applications that must use a timer should wait this value plus a small
// margin instead of a flat 15 seconds.
func (l *Link) EstablishmentTimeout() time.Duration {
	if l == nil {
		return time.Duration(EstablishmentTimeoutPerHop * float64(time.Second))
	}
	l.mutex.Lock()
	defer l.mutex.Unlock()
	return l.establishmentTimeout
}
