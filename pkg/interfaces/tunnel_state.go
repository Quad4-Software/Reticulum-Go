// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package interfaces

// Tunnel/filter/hop-limiter state added to BaseInterface for RNS 1.6.1:
// filtered byte counters, filtered announce counts, per-interface hop
// limiter, and tunnel synthesis/remote-transport metadata.

// TunnelsAllowed reports whether tunnel synthesis is accepted on this
// interface (Python Interface.tunnels_allowed).
func (b *BaseInterface) TunnelsAllowed() bool {
	b.Mutex.RLock()
	defer b.Mutex.RUnlock()
	return b.tunnelsAllowed
}

// SetTunnelsAllowed enables tunnel synthesis acceptance.
func (b *BaseInterface) SetTunnelsAllowed(v bool) {
	b.Mutex.Lock()
	b.tunnelsAllowed = v
	b.Mutex.Unlock()
}

// LosslessMedium reports the RNS 1.6.1 lossless_medium flag (set on TCP,
// backbone and I2P interfaces).
func (b *BaseInterface) LosslessMedium() bool {
	b.Mutex.RLock()
	defer b.Mutex.RUnlock()
	return b.losslessMedium
}

// SetLosslessMedium sets the lossless_medium flag.
func (b *BaseInterface) SetLosslessMedium(v bool) {
	b.Mutex.Lock()
	b.losslessMedium = v
	b.Mutex.Unlock()
}

// SynthesisEmitted reports whether a tunnel synthesis packet was sent.
func (b *BaseInterface) SynthesisEmitted() bool {
	b.Mutex.RLock()
	defer b.Mutex.RUnlock()
	return b.synthesisEmitted
}

// SetSynthesisEmitted records emitted synthesis state.
func (b *BaseInterface) SetSynthesisEmitted(v bool) {
	b.Mutex.Lock()
	b.synthesisEmitted = v
	b.Mutex.Unlock()
}

// SynthesisReceived reports whether a valid synthesis was received.
func (b *BaseInterface) SynthesisReceived() bool {
	b.Mutex.RLock()
	defer b.Mutex.RUnlock()
	return b.synthesisReceived
}

// SetSynthesisReceived records received synthesis state.
func (b *BaseInterface) SetSynthesisReceived(v bool) {
	b.Mutex.Lock()
	b.synthesisReceived = v
	b.Mutex.Unlock()
}

// RemoteTransportHash returns the remote tunnel transport identity hash
// (Python interface.remote_transport.hash), nil when no tunnel is up.
func (b *BaseInterface) RemoteTransportHash() []byte {
	b.Mutex.RLock()
	defer b.Mutex.RUnlock()
	return append([]byte(nil), b.remoteTransportHash...)
}

// SetRemoteTransportHash records the remote tunnel transport identity hash.
func (b *BaseInterface) SetRemoteTransportHash(h []byte) {
	b.Mutex.Lock()
	b.remoteTransportHash = append(b.remoteTransportHash[:0], h...)
	b.Mutex.Unlock()
}

// LocalTunnelID returns the locally originated tunnel ID, if any.
func (b *BaseInterface) LocalTunnelID() []byte {
	b.Mutex.RLock()
	defer b.Mutex.RUnlock()
	return append([]byte(nil), b.localTunnelID...)
}

// SetLocalTunnelID records the locally originated tunnel ID.
func (b *BaseInterface) SetLocalTunnelID(id []byte) {
	b.Mutex.Lock()
	b.localTunnelID = append(b.localTunnelID[:0], id...)
	b.Mutex.Unlock()
}

// SetRemoteTunnelMeta records the remote transport implementation metadata
// signalled in the tunnel synthesis ext payload.
func (b *BaseInterface) SetRemoteTunnelMeta(impl, vers string, isTN bool, dstCost, prCost float64) {
	b.Mutex.Lock()
	b.remoteImpl = impl
	b.remoteVers = vers
	b.remoteIsTN = isTN
	b.remoteDstCost = dstCost
	b.remotePRCost = prCost
	b.remoteMeta = true
	b.Mutex.Unlock()
}

// RemoteTunnelMeta returns the recorded remote transport metadata. ok is
// false when no tunnel synthesis metadata was received.
func (b *BaseInterface) RemoteTunnelMeta() (impl, vers string, isTN, ok bool, dstCost, prCost float64) {
	b.Mutex.RLock()
	defer b.Mutex.RUnlock()
	return b.remoteImpl, b.remoteVers, b.remoteIsTN, b.remoteMeta, b.remoteDstCost, b.remotePRCost
}

// AcceptedRxTally adds n accepted inbound bytes (Python rxb), counted only
// after transport accepts the frame. The parent interface tally happens in
// transport where the parent is reachable.
func (b *BaseInterface) AcceptedRxTally(n int) {
	b.Mutex.Lock()
	b.RxBytes += uint64(n) // #nosec G115 -- n is a byte count, non-negative
	b.Mutex.Unlock()
}

// FilteredRxTally adds n filtered inbound bytes (Python frxb). The parent
// interface tally happens in transport where the parent is reachable.
func (b *BaseInterface) FilteredRxTally(n int) {
	b.Mutex.Lock()
	b.frxb += uint64(n) // #nosec G115 -- n is a byte count, non-negative
	b.Mutex.Unlock()
}

// FilteredTxTally adds n filtered outbound bytes (Python ftxb).
func (b *BaseInterface) FilteredTxTally(n int) {
	b.Mutex.Lock()
	b.ftxb += uint64(n) // #nosec G115 -- n is a byte count, non-negative
	b.Mutex.Unlock()
}

// FilteredRxB returns filtered received bytes.
func (b *BaseInterface) FilteredRxB() uint64 {
	b.Mutex.RLock()
	defer b.Mutex.RUnlock()
	return b.frxb
}

// FilteredTxB returns filtered transmitted bytes.
func (b *BaseInterface) FilteredTxB() uint64 {
	b.Mutex.RLock()
	defer b.Mutex.RUnlock()
	return b.ftxb
}

// FilteredAnnounce increments the filtered announce count (Python afrxc).
func (b *BaseInterface) FilteredAnnounce() {
	b.Mutex.Lock()
	b.afrxc++
	b.Mutex.Unlock()
}

// FilteredAnnounceCount returns the filtered announce count.
func (b *BaseInterface) FilteredAnnounceCount() uint64 {
	b.Mutex.RLock()
	defer b.Mutex.RUnlock()
	return b.afrxc
}

// FilteredSpeeds returns the last sampled filtered rx/tx speeds (bit/s).
func (b *BaseInterface) FilteredSpeeds() (float64, float64) {
	b.Mutex.RLock()
	defer b.Mutex.RUnlock()
	return b.currentFRXS, b.currentFTXS
}

// SetHopLimiter installs the per-interface announce hop limit
// (Python interface.hop_limiter). n <= 0 means no per-interface limit.
func (b *BaseInterface) SetHopLimiter(n int) {
	b.Mutex.Lock()
	if n > 0 {
		b.hopLimiter = n
		b.hopLimiterSet = true
	} else {
		b.hopLimiter = 0
		b.hopLimiterSet = false
	}
	b.Mutex.Unlock()
}

// HopLimiter returns the configured per-interface hop limit.
func (b *BaseInterface) HopLimiter() (int, bool) {
	b.Mutex.RLock()
	defer b.Mutex.RUnlock()
	return b.hopLimiter, b.hopLimiterSet
}

// HopLimiterHit increments the per-interface hop-limiter drop counter.
func (b *BaseInterface) HopLimiterHit() {
	b.Mutex.Lock()
	b.hopLimiterHits++
	b.Mutex.Unlock()
}

// HopLimiterHits returns the per-interface hop-limiter drop count.
func (b *BaseInterface) HopLimiterHits() uint64 {
	b.Mutex.RLock()
	defer b.Mutex.RUnlock()
	return b.hopLimiterHits
}
