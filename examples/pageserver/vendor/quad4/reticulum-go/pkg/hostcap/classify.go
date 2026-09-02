// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package hostcap

// Thresholds tuned from transport bench data and RNS public-node reports.
// Slow VPS nodes often sit around 1 to 4 GB/s memcpy while desktops exceed 10.
const (
	memCopyWarnGBps  = 4.0
	memCopyErrorGBps = 1.0
	cpuWarnMBps      = 400.0
	cpuErrorMBps     = 100.0
)

func classify(memGBps, cpuMBps float64) (memClass, cpuClass Class) {
	memClass = ClassOK
	switch {
	case memGBps < memCopyErrorGBps:
		memClass = ClassError
	case memGBps < memCopyWarnGBps:
		memClass = ClassWarn
	}
	cpuClass = ClassOK
	switch {
	case cpuMBps < cpuErrorMBps:
		cpuClass = ClassError
	case cpuMBps < cpuWarnMBps:
		cpuClass = ClassWarn
	}
	return memClass, cpuClass
}

// MattersForTransport reports whether the probe suggests hardware limits for relay.
func (r Report) MattersForTransport() bool {
	if !r.Transport {
		return false
	}
	return r.MemClass >= ClassWarn || r.CPUClass >= ClassWarn
}
