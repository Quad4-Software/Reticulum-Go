// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package hostcap

import (
	"quad4/reticulum-go/pkg/debug"
)

// LogUnderMemoryPressure adds host-capability context when heap shedding starts.
func LogUnderMemoryPressure(heapBytes, limitBytes uint64) {
	r := LastReport()
	if r == nil {
		return
	}
	if r.MemClass < ClassWarn {
		return
	}
	debug.Log(debug.DebugWarning, "Memory pressure on host with slow memory bandwidth",
		"heap_bytes", heapBytes,
		"limit_bytes", limitBytes,
		"memcopy_gbps", round2(r.MemCopyGBps),
		"hint", "heap growth and packet copies compete for the same limited bandwidth on this host")
}

// Log writes probe results through the debug logger.
func Log(r Report) {
	debug.Log(debug.DebugInfo, "Host capability probe",
		"memcopy_gbps", round2(r.MemCopyGBps),
		"cpu_mbps", round2(r.CPUMbps),
		"num_cpu", r.NumCPU,
		"gomaxprocs", r.GOMAXPROCS,
		"transport", r.Transport,
		"probe_ms", r.Duration.Milliseconds())

	switch r.MemClass {
	case ClassWarn:
		debug.Log(debug.DebugWarning, "Slow host memory bandwidth",
			"memcopy_gbps", round2(r.MemCopyGBps),
			"warn_below_gbps", memCopyWarnGBps,
			"hint", "packet relay and IFAC masking copy every byte. Public transport nodes may see higher latency under load")
	case ClassError:
		debug.Log(debug.DebugError, "Very slow host memory bandwidth",
			"memcopy_gbps", round2(r.MemCopyGBps),
			"error_below_gbps", memCopyErrorGBps,
			"hint", "this host is likely to struggle as a busy transport node")
	}

	switch r.CPUClass {
	case ClassWarn:
		debug.Log(debug.DebugWarning, "Slow host CPU throughput",
			"cpu_mbps", round2(r.CPUMbps),
			"warn_below_mbps", cpuWarnMBps,
			"hint", "inbound queue draining and crypto may lag on sustained traffic")
	case ClassError:
		debug.Log(debug.DebugError, "Very slow host CPU throughput",
			"cpu_mbps", round2(r.CPUMbps),
			"error_below_mbps", cpuErrorMBps,
			"hint", "consider fewer interfaces, lower MTU, or a larger instance")
	}

	if r.MattersForTransport() {
		debug.Log(debug.DebugWarning, "Host hardware may limit Reticulum transport throughput",
			"memcopy_gbps", round2(r.MemCopyGBps),
			"cpu_mbps", round2(r.CPUMbps))
	} else if r.Transport && debug.Enabled(debug.DebugVerbose) {
		debug.Log(debug.DebugVerbose, "Host capability adequate for transport relay",
			"memcopy_gbps", round2(r.MemCopyGBps),
			"cpu_mbps", round2(r.CPUMbps))
	}
}

func round2(v float64) float64 {
	return float64(int(v*100+0.5)) / 100
}
