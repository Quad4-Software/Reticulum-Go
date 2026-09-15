// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package interfaces

import "time"

// ProtocolViolation records a protocol violation on this interface.
func (i *BaseInterface) ProtocolViolation() {
	if i == nil {
		return
	}
	i.Mutex.Lock()
	i.protocolViolations++
	i.Mutex.Unlock()
}

// IFACViolation records an IFAC violation on this interface.
func (i *BaseInterface) IFACViolation() {
	if i == nil {
		return
	}
	i.Mutex.Lock()
	i.ifacViolations++
	i.Mutex.Unlock()
}

// PacketFilterHit records a packet filter drop on this interface.
func (i *BaseInterface) PacketFilterHit() {
	if i == nil {
		return
	}
	i.Mutex.Lock()
	i.packetFilterHits++
	i.Mutex.Unlock()
}

// ProtocolViolations returns the protocol violation counter.
func (i *BaseInterface) ProtocolViolations() uint64 {
	if i == nil {
		return 0
	}
	i.Mutex.RLock()
	defer i.Mutex.RUnlock()
	return i.protocolViolations
}

// IFACViolations returns the IFAC violation counter.
func (i *BaseInterface) IFACViolations() uint64 {
	if i == nil {
		return 0
	}
	i.Mutex.RLock()
	defer i.Mutex.RUnlock()
	return i.ifacViolations
}

// PacketFilterHits returns the packet filter hit counter.
func (i *BaseInterface) PacketFilterHits() uint64 {
	if i == nil {
		return 0
	}
	i.Mutex.RLock()
	defer i.Mutex.RUnlock()
	return i.packetFilterHits
}

// AnnounceRXBytes returns announce bytes received on this interface.
func (i *BaseInterface) AnnounceRXBytes() uint64 {
	if i == nil {
		return 0
	}
	i.Mutex.RLock()
	defer i.Mutex.RUnlock()
	return i.arxb
}

// AnnounceTXBytes returns announce bytes sent on this interface.
func (i *BaseInterface) AnnounceTXBytes() uint64 {
	if i == nil {
		return 0
	}
	i.Mutex.RLock()
	defer i.Mutex.RUnlock()
	return i.atxb
}

// AnnounceRXCount returns announce packets received on this interface.
func (i *BaseInterface) AnnounceRXCount() uint64 {
	if i == nil {
		return 0
	}
	i.Mutex.RLock()
	defer i.Mutex.RUnlock()
	return i.arxc
}

// AnnounceTXCount returns announce packets sent on this interface.
func (i *BaseInterface) AnnounceTXCount() uint64 {
	if i == nil {
		return 0
	}
	i.Mutex.RLock()
	defer i.Mutex.RUnlock()
	return i.atxc
}

// PathRequestRXBytes returns path-request bytes received on this interface.
func (i *BaseInterface) PathRequestRXBytes() uint64 {
	if i == nil {
		return 0
	}
	i.Mutex.RLock()
	defer i.Mutex.RUnlock()
	return i.prxb
}

// PathRequestTXBytes returns path-request bytes sent on this interface.
func (i *BaseInterface) PathRequestTXBytes() uint64 {
	if i == nil {
		return 0
	}
	i.Mutex.RLock()
	defer i.Mutex.RUnlock()
	return i.ptxb
}

// PathRequestRXCount returns path-request packets received on this interface.
func (i *BaseInterface) PathRequestRXCount() uint64 {
	if i == nil {
		return 0
	}
	i.Mutex.RLock()
	defer i.Mutex.RUnlock()
	return i.prxc
}

// PathRequestTXCount returns path-request packets sent on this interface.
func (i *BaseInterface) PathRequestTXCount() uint64 {
	if i == nil {
		return 0
	}
	i.Mutex.RLock()
	defer i.Mutex.RUnlock()
	return i.ptxc
}

// AnnounceRXSpeed returns the estimated incoming announce throughput in bps.
func (i *BaseInterface) AnnounceRXSpeed() float64 {
	if i == nil {
		return 0
	}
	i.Mutex.RLock()
	defer i.Mutex.RUnlock()
	return i.currentARXS
}

// AnnounceTXSpeed returns the estimated outgoing announce throughput in bps.
func (i *BaseInterface) AnnounceTXSpeed() float64 {
	if i == nil {
		return 0
	}
	i.Mutex.RLock()
	defer i.Mutex.RUnlock()
	return i.currentATXS
}

// PathRequestRXSpeed returns the estimated incoming path-request throughput in bps.
func (i *BaseInterface) PathRequestRXSpeed() float64 {
	if i == nil {
		return 0
	}
	i.Mutex.RLock()
	defer i.Mutex.RUnlock()
	return i.currentPRXS
}

// PathRequestTXSpeed returns the estimated outgoing path-request throughput in bps.
func (i *BaseInterface) PathRequestTXSpeed() float64 {
	if i == nil {
		return 0
	}
	i.Mutex.RLock()
	defer i.Mutex.RUnlock()
	return i.currentPTXS
}

func (i *BaseInterface) tallyReceivedAnnounceBytes(size int) {
	i.arxc++
	if size > 0 {
		i.arxb += uint64(size)
	}
}

func (i *BaseInterface) tallySentAnnounceBytes(size int) {
	i.atxc++
	if size > 0 {
		i.atxb += uint64(size)
	}
}

func (i *BaseInterface) tallyReceivedPathRequestBytes(size int) {
	i.prxc++
	if size > 0 {
		i.prxb += uint64(size)
	}
}

func (i *BaseInterface) tallySentPathRequestBytes(size int) {
	i.ptxc++
	if size > 0 {
		i.ptxb += uint64(size)
	}
}

func (i *BaseInterface) sampleTypedTraffic(now time.Time, elapsed float64) {
	if elapsed <= 0 {
		return
	}
	arDiff := i.arxb - i.sampleARXB
	atDiff := i.atxb - i.sampleATXB
	prDiff := i.prxb - i.samplePRXB
	ptDiff := i.ptxb - i.samplePTXB
	i.currentARXS = float64(arDiff*8) / elapsed
	i.currentATXS = float64(atDiff*8) / elapsed
	i.currentPRXS = float64(prDiff*8) / elapsed
	i.currentPTXS = float64(ptDiff*8) / elapsed
	i.sampleARXB = i.arxb
	i.sampleATXB = i.atxb
	i.samplePRXB = i.prxb
	i.samplePTXB = i.ptxb
	_ = now
}
