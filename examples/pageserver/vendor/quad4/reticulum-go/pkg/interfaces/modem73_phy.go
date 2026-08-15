// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package interfaces

import "math"

// PHY tables transcribed from RFnexus/modem73 (tnc_ui.hh, phy/robust_modem.hh,
// phy/mfsk_modem.hh, CONTROL_PORT.md). payload_size is the PHY capacity including
// the 2-byte length prefix that modem73 reports via get_config.

const (
	modem73RobustModeCount = 12
	modem73SampleRate      = 48000
	modem73RobustSYM       = 960
)

var modem73RobustBPSExt = []int{
	1150, 585, 296, 296, 149,
	732, 378, 194, 197, 99,
	780, 510,
}

var modem73RobustNRows = []int{
	173, 345, 685, 685, 1369,
	89, 177, 349, 345, 689,
	257, 129,
}

var modem73RobustTimeoutBPSExt = []int{
	295, 100, 75, 75, 38,
	185, 95, 50, 50, 25,
	200, 130,
}

var modem73OFDMPayloadShort = [8][5]int{
	{128, 171, 192, 213, 64},
	{128, 171, 192, 213, 64},
	{512, 684, 768, 852, 256},
	{256, 342, 384, 426, 128},
	{1024, 1368, 1536, 1704, 512},
	{1024, 1368, 1536, 1704, 512},
	{2048, 2736, 3072, 3408, 1024},
	{2048, 2736, 3072, 3408, 1024},
}

var modem73OFDMPayloadNormal = [8][5]int{
	{256, 342, 384, 426, 128},
	{512, 684, 768, 852, 256},
	{1024, 1368, 1536, 1704, 512},
	{1024, 1368, 1536, 1704, 512},
	{2048, 2736, 3072, 3408, 1024},
	{2048, 2736, 3072, 3408, 1024},
	{4096, 5472, 6144, 6816, 2048},
	{4096, 5472, 6144, 6816, 2048},
}

var modem73OFDMPayloadLong = [8][5]int{
	{512, 684, 768, 852, 256},
	{1024, 1368, 1536, 1704, 512},
	{2048, 2736, 3072, 3408, 1024},
	{2048, 2736, 3072, 3408, 1024},
	{4096, 5472, 6144, 6816, 2048},
	{4096, 5472, 6144, 6816, 2048},
	{0, 0, 0, 0, 0},
	{0, 0, 0, 0, 0},
}

var modem73OFDMBitrateShort = [8][5]int{
	{700, 900, 1000, 1100, 300},
	{1100, 1400, 1600, 1800, 500},
	{2100, 2900, 3200, 3600, 1100},
	{2100, 2900, 3200, 3600, 1000},
	{4300, 5700, 6400, 7100, 2200},
	{5400, 7300, 8200, 9100, 2700},
	{7500, 10000, 11200, 12500, 3700},
	{8600, 11400, 12800, 14200, 4300},
}

var modem73OFDMBitrateNormal = [8][5]int{
	{800, 1100, 1200, 1300, 400},
	{1600, 2100, 2400, 2600, 800},
	{2400, 3200, 3600, 4000, 1200},
	{3200, 4200, 4700, 5200, 1600},
	{4800, 6400, 7200, 8000, 2400},
	{6300, 8400, 9500, 10500, 3200},
	{8300, 11000, 12400, 13800, 4100},
	{9600, 12800, 14400, 16000, 4800},
}

var modem73OFDMBitrateLong = [8][5]int{
	{856, 1144, 1285, 1425, 428},
	{1713, 2288, 2569, 2850, 856},
	{2551, 3408, 3826, 4245, 1275},
	{3425, 4576, 5138, 5700, 1713},
	{5101, 6815, 7652, 8489, 2551},
	{6851, 9152, 10276, 11400, 3425},
	{0, 0, 0, 0, 0},
	{0, 0, 0, 0, 0},
}

var modem73OFDMDurationShortMs = [8]int{1500, 1000, 1900, 1000, 1900, 1500, 2200, 1900}
var modem73OFDMDurationNormalMs = [8]int{2600, 2600, 3400, 2600, 3400, 2600, 4000, 3400}
var modem73OFDMDurationLongMs = [8]int{4800, 4800, 6400, 4800, 6400, 4800, 0, 0}

var modem73OFDMRepPayloadShort = [8]int{128, 256, 512, 512, 1024, 1024, 2048, 2048}
var modem73OFDMRepPayloadNormal = [8]int{256, 512, 1024, 1024, 2048, 0, 0, 0}
var modem73OFDMRepDurationShortMs = [8]int{2600, 1500, 3400, 1500, 3400, 2600, 4000, 3400}
var modem73OFDMRepDurationNormalMs = [8]int{4800, 4800, 6400, 4800, 6400, 0, 0, 0}

var modem73MFSKPayload = []int{32, 64, 128, 96}
var modem73MFSKDurationS = 8.0

// Modem73PhyResult is the computed PHY view for a TNC configuration.
type Modem73PhyResult struct {
	PayloadSize int
	BitrateBPS  int
	AirtimeS    float64
	MTUBytes    int
}

// Modem73RobustAirtime returns frame duration from nrows and sample clock.
func Modem73RobustAirtime(mode int) float64 {
	if mode < 0 || mode >= len(modem73RobustNRows) {
		return 0
	}
	syms := 5 + modem73RobustNRows[mode]
	return float64(syms) * float64(modem73RobustSYM) / float64(modem73SampleRate)
}

// Modem73RobustPayload returns PHY payload_size for a robust mode.
func Modem73RobustPayload(mode int) int {
	if mode < 0 || mode >= modem73RobustModeCount {
		return 0
	}
	if (mode >= 5 && mode < 10) || mode == 11 {
		return 172
	}
	return 512
}

// Modem73ComputePhy derives payload bitrate and airtime for a modem config.
func Modem73ComputePhy(modemType, robustMode, mfskMode, frameSize int, modulation, codeRate string) Modem73PhyResult {
	switch modemType {
	case modem73TypeRobust:
		ps := Modem73RobustPayload(robustMode)
		air := Modem73RobustAirtime(robustMode)
		bps := 0
		if robustMode >= 0 && robustMode < len(modem73RobustBPSExt) {
			bps = modem73RobustBPSExt[robustMode]
		} else if air > 0 {
			bps = int(float64(ps*8) / air)
		}
		return Modem73PhyResult{PayloadSize: ps, BitrateBPS: bps, AirtimeS: air, MTUBytes: ps - 2}
	case modem73TypeMFSK:
		if mfskMode < 0 || mfskMode >= len(modem73MFSKPayload) {
			mfskMode = 0
		}
		ps := modem73MFSKPayload[mfskMode] + 4
		air := modem73MFSKDurationS
		bps := int(float64(modem73MFSKPayload[mfskMode]*8) / air)
		return Modem73PhyResult{PayloadSize: ps, BitrateBPS: bps, AirtimeS: air, MTUBytes: modem73MFSKPayload[mfskMode]}
	default:
		return modem73OFDMPhy(modulation, codeRate, frameSize)
	}
}

func modem73OFDMPhy(modulation, codeRate string, frameSize int) Modem73PhyResult {
	mod := modem73IndexOf(modem73OFDMModulations, modulation)
	rate := modem73IndexOf(modem73OFDMCodeRates, codeRate)
	if mod < 0 {
		mod = 1
	}
	if rate < 0 {
		rate = 0
	}
	if frameSize < 0 {
		frameSize = 1
	}
	if frameSize > 2 {
		frameSize = 2
	}

	if rate >= 5 {
		var pl, du int
		if frameSize == 0 {
			pl = modem73OFDMRepPayloadShort[mod]
			du = modem73OFDMRepDurationShortMs[mod]
		} else if frameSize == 1 {
			pl = modem73OFDMRepPayloadNormal[mod]
			du = modem73OFDMRepDurationNormalMs[mod]
		}
		air := float64(du) / 1000
		bps := 0
		if du > 0 {
			bps = int(float64(pl) * 8000 / float64(du))
		}
		mtu := 0
		if pl > 0 {
			mtu = pl - 2
		}
		return Modem73PhyResult{PayloadSize: pl, BitrateBPS: bps, AirtimeS: air, MTUBytes: mtu}
	}

	var pl, bps, du int
	switch frameSize {
	case 0:
		pl = modem73OFDMPayloadShort[mod][rate]
		bps = modem73OFDMBitrateShort[mod][rate]
		du = modem73OFDMDurationShortMs[mod]
	case 2:
		pl = modem73OFDMPayloadLong[mod][rate]
		bps = modem73OFDMBitrateLong[mod][rate]
		du = modem73OFDMDurationLongMs[mod]
	default:
		pl = modem73OFDMPayloadNormal[mod][rate]
		bps = modem73OFDMBitrateNormal[mod][rate]
		du = modem73OFDMDurationNormalMs[mod]
	}
	air := float64(du) / 1000
	mtu := 0
	if pl > 0 {
		mtu = pl - 2
	}
	return Modem73PhyResult{PayloadSize: pl, BitrateBPS: bps, AirtimeS: air, MTUBytes: mtu}
}

// Modem73BPSKBER returns approximate AWGN BPSK BER for SNR in dB.
func Modem73BPSKBER(snrDB float64) float64 {
	if snrDB < -20 {
		return 0.5
	}
	snrLin := math.Pow(10, snrDB/10)
	return 0.5 * math.Erfc(math.Sqrt(snrLin))
}

// Modem73FrameErrorRate maps bit BER to approximate frame error rate.
func Modem73FrameErrorRate(ber float64, payloadBytes int) float64 {
	if ber <= 0 {
		return 0
	}
	if ber >= 0.5 {
		return 1
	}
	n := float64(payloadBytes * 8)
	return 1 - math.Pow(1-ber, n)
}
