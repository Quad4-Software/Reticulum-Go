// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package interfaces

import "math"

const (
	modem73TypeOFDM   = 0
	modem73TypeMFSK   = 1
	modem73TypeRobust = 2

	modem73RobustShortOffset = 5
	modem73RobustModeMax     = 11

	modem73CSMAKeyupS     = 0.65
	modem73CSMABurstGapS  = 0.2
	modem73MFSKTimeoutBPS = 10
)

// Legacy aliases kept for existing call sites. Prefer modem73RobustBPSExt.
var modem73RobustBPS = modem73RobustBPSExt
var modem73RobustTimeoutBPS = modem73RobustTimeoutBPSExt

// modem73RobustAirtime legacy slice derived from nrows formula for modes 0-9.
var modem73RobustAirtime = func() []float64 {
	out := make([]float64, 10)
	for i := range 10 {
		out[i] = Modem73RobustAirtime(i)
	}
	return out
}()

var modem73OFDMAirtime = map[string][3]float64{
	"BPSK":    {1.50, 2.60, 4.80},
	"QPSK":    {1.00, 2.60, 4.80},
	"8PSK":    {1.90, 3.40, 6.40},
	"QAM16":   {1.00, 2.60, 4.80},
	"QAM64":   {1.90, 3.40, 6.40},
	"QAM256":  {1.50, 2.60, 4.80},
	"QAM1024": {2.20, 4.00, 4.00},
	"QAM4096": {1.90, 3.40, 3.40},
}

var modem73OFDMModulations = []string{
	"BPSK", "QPSK", "8PSK", "QAM16", "QAM64", "QAM256", "QAM1024", "QAM4096",
}

var modem73OFDMCodeRates = []string{
	"1/2", "2/3", "3/4", "5/6", "1/4", "1/2x2", "1/4x2",
}

type modem73PhyProfile struct {
	phyBPS  float64
	airtime float64
}

func modem73CfgInt(cfg map[string]any, key string) (int, bool) {
	v, ok := cfg[key]
	if !ok || v == nil {
		return 0, false
	}
	switch n := v.(type) {
	case float64:
		return int(n), true
	case int:
		return n, true
	case int64:
		return int(n), true
	case jsonNumber:
		i, err := n.Int64()
		if err != nil {
			return 0, false
		}
		return int(i), true
	default:
		return 0, false
	}
}

// jsonNumber is satisfied by encoding/json.Number without importing it in callers.
type jsonNumber interface {
	Int64() (int64, error)
}

func modem73CfgFloat(cfg map[string]any, key string) (float64, bool) {
	v, ok := cfg[key]
	if !ok || v == nil {
		return 0, false
	}
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	default:
		return 0, false
	}
}

func modem73CfgBool(cfg map[string]any, key string, def bool) bool {
	v, ok := cfg[key]
	if !ok || v == nil {
		return def
	}
	b, ok := v.(bool)
	if !ok {
		return def
	}
	return b
}

func modem73CfgString(cfg map[string]any, key string) string {
	v, ok := cfg[key]
	if !ok || v == nil {
		return ""
	}
	s, _ := v.(string)
	return s
}

func modem73PhyProfileFromCfg(cfg map[string]any) (modem73PhyProfile, bool) {
	mt, ok := modem73CfgInt(cfg, "modem_type")
	if !ok {
		return modem73PhyProfile{}, false
	}
	switch mt {
	case modem73TypeRobust:
		rm, ok := modem73CfgInt(cfg, "robust_mode")
		if !ok || rm < 0 || rm > modem73RobustModeMax {
			return modem73PhyProfile{}, false
		}
		air := Modem73RobustAirtime(rm)
		bps := float64(modem73RobustBPSExt[rm])
		return modem73PhyProfile{phyBPS: bps, airtime: air}, true
	case modem73TypeOFDM:
		mod := modem73CfgString(cfg, "modulation")
		code := modem73CfgString(cfg, "code_rate")
		fs, ok := modem73CfgInt(cfg, "frame_size")
		if !ok {
			if modem73CfgBool(cfg, "short_frame", false) {
				fs = 0
			} else {
				fs = 1
			}
		}
		phy := modem73OFDMPhy(mod, code, fs)
		if phy.PayloadSize <= 0 || phy.AirtimeS <= 0 {
			return modem73PhyProfile{}, false
		}
		return modem73PhyProfile{
			phyBPS:  float64(phy.BitrateBPS),
			airtime: phy.AirtimeS,
		}, true
	default:
		return modem73PhyProfile{}, false
	}
}

func modem73CSMAPerFrameOverhead(cfg map[string]any, airtime float64) float64 {
	if !modem73CfgBool(cfg, "csma_enabled", true) {
		return 0
	}
	quietMs, ok := modem73CfgFloat(cfg, "csma_quiet_ms")
	if !ok || quietMs <= 0 {
		quietMs = math.Min(math.Max(airtime*250.0, 300.0), 3500.0)
	}
	cw, ok := modem73CfgInt(cfg, "csma_cw")
	if !ok || cw < 2 {
		cw = 8
	}
	slotMs, ok := modem73CfgInt(cfg, "slot_time_ms")
	if !ok || slotMs < 1 {
		slotMs = 500
	}
	burst, ok := modem73CfgInt(cfg, "csma_burst")
	if !ok || burst < 1 {
		burst = 1
	}
	if burst > 4 {
		burst = 4
	}
	access := quietMs/1000.0 + float64(cw*slotMs)/2000.0 + modem73CSMAKeyupS
	return access/float64(burst) + modem73CSMABurstGapS*float64(burst-1)/float64(burst)
}

// modem73TimeoutBitrate returns an effective bitrate for path timing.
func modem73TimeoutBitrate(cfg map[string]any, csmaOverhead bool, timeoutMargin float64) (int, bool) {
	mt, ok := modem73CfgInt(cfg, "modem_type")
	if ok && mt == modem73TypeMFSK {
		return modem73MFSKTimeoutBPS, true
	}
	if !csmaOverhead {
		if ok && mt == modem73TypeRobust {
			rm, rok := modem73CfgInt(cfg, "robust_mode")
			if rok && rm >= 0 && rm <= modem73RobustModeMax {
				return modem73RobustTimeoutBPS[rm], true
			}
		}
		return 0, false
	}
	prof, ok := modem73PhyProfileFromCfg(cfg)
	if !ok {
		return 0, false
	}
	overhead := modem73CSMAPerFrameOverhead(cfg, prof.airtime)
	duty := 1.0
	if overhead > 0 {
		duty = prof.airtime / (prof.airtime + overhead)
	}
	bps := max(int(prof.phyBPS*duty*timeoutMargin), 8)
	return bps, true
}

func modem73IndexOf(list []string, v string) int {
	for i, s := range list {
		if s == v {
			return i
		}
	}
	return -1
}

// modem73ShortOperMode returns a control-port oper_mode override when short frames apply.
func modem73ShortOperMode(cfg map[string]any) (int, bool) {
	mt, ok := modem73CfgInt(cfg, "modem_type")
	if !ok {
		return 0, false
	}
	switch mt {
	case modem73TypeRobust:
		rm, ok := modem73CfgInt(cfg, "robust_mode")
		if !ok || rm >= modem73RobustShortOffset {
			return 0, false
		}
		return rm + modem73RobustShortOffset, true
	case modem73TypeOFDM:
		if modem73CfgBool(cfg, "short_frame", false) {
			return 0, false
		}
		mod := modem73IndexOf(modem73OFDMModulations, modem73CfgString(cfg, "modulation"))
		rate := modem73IndexOf(modem73OFDMCodeRates, modem73CfgString(cfg, "code_rate"))
		if mod < 0 || rate < 0 {
			return 0, false
		}
		return (mod << 4) | (rate << 1), true
	default:
		return 0, false
	}
}

// modem73PathRequestTimeoutSec sizes the path-request window for a given bitrate.
func modem73PathRequestTimeoutSec(bitrate int64, mtu int) int {
	if bitrate <= 0 || mtu <= 0 {
		return 0
	}
	return int(3*(float64(mtu)*8/float64(bitrate))) + 10
}
