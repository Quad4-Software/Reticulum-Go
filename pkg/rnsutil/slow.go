// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package rnsutil

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"sort"
	"strings"
	"time"

	"quad4/reticulum-go/pkg/term"
	"quad4/reticulum-go/pkg/transport"
)

// Severity ranks a bottleneck finding.
type Severity string

const (
	SeverityInfo     Severity = "info"
	SeverityWarn     Severity = "warn"
	SeverityCritical Severity = "critical"
)

// SlowFinding is one scored bottleneck or observation.
type SlowFinding struct {
	Kind     string   `json:"kind"` // interface|path|transport|destination
	Name     string   `json:"name"`
	Severity Severity `json:"severity"`
	Score    float64  `json:"score"`
	Summary  string   `json:"summary"`
	Detail   string   `json:"detail,omitempty"`
	Hints    []string `json:"hints,omitempty"`
}

// PathEgressHotspot aggregates paths leaving via one interface.
type PathEgressHotspot struct {
	Interface string  `json:"interface"`
	Paths     int     `json:"paths"`
	AvgHops   float64 `json:"avg_hops"`
	MaxHops   int     `json:"max_hops"`
	HighHop   int     `json:"high_hop_paths"` // hops >= HighHopThreshold
}

// DestFocus is optional per-destination routing context.
type DestFocus struct {
	Hash             string  `json:"hash"`
	HasPath          bool    `json:"has_path"`
	Hops             int     `json:"hops,omitempty"`
	Via              string  `json:"via,omitempty"`
	Interface        string  `json:"interface,omitempty"`
	FirstHopTimeoutS float64 `json:"first_hop_timeout_s,omitempty"`
	ExpiresInS       float64 `json:"expires_in_s,omitempty"`
}

// SlowReport is the full bottleneck diagnosis.
type SlowReport struct {
	GeneratedAt     time.Time           `json:"generated_at"`
	RPCAddr         string              `json:"rpc_addr"`
	TransportID     string              `json:"transport_id,omitempty"`
	TransportUptime float64             `json:"transport_uptime_s"`
	LinkCount       *int                `json:"link_count,omitempty"`
	Totals          SlowTrafficTotals   `json:"totals"`
	Interfaces      []SlowIfaceRow      `json:"interfaces"`
	EgressHotspots  []PathEgressHotspot `json:"egress_hotspots"`
	Findings        []SlowFinding       `json:"findings"`
	Recommendations []string            `json:"recommendations"`
	Destination     *DestFocus          `json:"destination,omitempty"`
	PathStats       SlowPathStats       `json:"path_stats"`
	HighHopAt       int                 `json:"high_hop_threshold"`
}

// SlowTrafficTotals summarizes transport-wide rates.
type SlowTrafficTotals struct {
	RXB uint64  `json:"rxb"`
	TXB uint64  `json:"txb"`
	RXS float64 `json:"rxs"`
	TXS float64 `json:"txs"`
}

// SlowPathStats summarizes the path table.
type SlowPathStats struct {
	Total      int     `json:"total"`
	HighHop    int     `json:"high_hop"` // hops >= HighHopThreshold
	VeryHigh   int     `json:"very_high_hop"`
	AvgHops    float64 `json:"avg_hops"`
	MaxHops    int     `json:"max_hops"`
	MedianHops int     `json:"median_hops"`
}

// SlowIfaceRow is a scored interface snapshot.
type SlowIfaceRow struct {
	Name               string   `json:"name"`
	Type               string   `json:"type"`
	Online             bool     `json:"online"`
	Bitrate            int64    `json:"bitrate"`
	RXS                float64  `json:"rxs"`
	TXS                float64  `json:"txs"`
	RXB                uint64   `json:"rxb"`
	TXB                uint64   `json:"txb"`
	UtilPct            float64  `json:"util_pct"`
	HeldAnnounces      int      `json:"held_announces"`
	BurstActive        bool     `json:"burst_active"`
	PRBurstActive      bool     `json:"pr_burst_active"`
	AnnounceHz         float64  `json:"announce_hz"`
	PRHz               float64  `json:"pr_hz"`
	RTTMs              *float64 `json:"rtt_ms,omitempty"`
	BandwidthAvailable *bool    `json:"bandwidth_available,omitempty"`
	PathCount          int      `json:"path_count"`
	IFACFail           uint64   `json:"ifac_fail"`
	HMACFail           uint64   `json:"hmac_fail"`
	AnnounceSigFail    uint64   `json:"announce_sig_fail"`
	UnpackFail         uint64   `json:"unpack_fail"`
	IntegrityFailRate  float64  `json:"integrity_fail_rate"`
	IntegritySamples60 uint64   `json:"integrity_samples_60s"`
	StaleCloses        uint64   `json:"stale_closes"`
	KeepaliveTimeout   uint64   `json:"keepalive_timeout"`
	ProofFail          uint64   `json:"proof_fail"`
	Score              float64  `json:"score"`
	Flags              []string `json:"flags"`
	Why                string   `json:"why"`
}

// SlowAnalyzeOptions tunes thresholds.
type SlowAnalyzeOptions struct {
	HighHopThreshold     int     // default 6
	VeryHighHopThreshold int     // default 10
	UtilWarnPct          float64 // default 60
	UtilCritPct          float64 // default 85
	MinBitrateForUtil    int64   // ignore util when bitrate below this (default 8kbps)
	TopN                 int     // interfaces to keep in report (default 12)
	NameFilter           string
	ShowAll              bool
	IntegrityWarnRate    float64 // default 0.05
	IntegrityCritRate    float64 // default 0.20
	MinIntegritySamples  uint64  // default 20 fails+ok in window proxy via totals
	StaleWarn            uint64  // default 3
	StaleCrit            uint64  // default 10
	AuthFailWarn         uint64  // default 5 announce_sig+proof fails
	AuthFailCrit         uint64  // default 25
}

func (o *SlowAnalyzeOptions) normalize() {
	if o.HighHopThreshold <= 0 {
		o.HighHopThreshold = 6
	}
	if o.VeryHighHopThreshold <= 0 {
		o.VeryHighHopThreshold = 10
	}
	if o.UtilWarnPct <= 0 {
		o.UtilWarnPct = 60
	}
	if o.UtilCritPct <= 0 {
		o.UtilCritPct = 85
	}
	if o.MinBitrateForUtil <= 0 {
		o.MinBitrateForUtil = 8_000
	}
	if o.TopN <= 0 {
		o.TopN = 12
	}
	if o.IntegrityWarnRate <= 0 {
		o.IntegrityWarnRate = 0.05
	}
	if o.IntegrityCritRate <= 0 {
		o.IntegrityCritRate = 0.20
	}
	if o.MinIntegritySamples == 0 {
		o.MinIntegritySamples = 20
	}
	if o.StaleWarn == 0 {
		o.StaleWarn = 3
	}
	if o.StaleCrit == 0 {
		o.StaleCrit = 10
	}
	if o.AuthFailWarn == 0 {
		o.AuthFailWarn = 5
	}
	if o.AuthFailCrit == 0 {
		o.AuthFailCrit = 25
	}
}

// AnalyzeSlow builds a bottleneck report from shared-instance RPC payloads.
// Works against Go and Python daemons that expose interface_stats + path_table.
func AnalyzeSlow(
	stats transport.InterfaceStatsResponse,
	paths []transport.PathTableEntry,
	linkCount *int,
	rpcAddr string,
	opts SlowAnalyzeOptions,
) SlowReport {
	opts.normalize()
	now := time.Now()
	rep := SlowReport{
		GeneratedAt:     now,
		RPCAddr:         rpcAddr,
		TransportID:     PrettyHex(stats.TransportID),
		TransportUptime: stats.TransportUptime,
		LinkCount:       linkCount,
		HighHopAt:       opts.HighHopThreshold,
		Totals: SlowTrafficTotals{
			RXB: stats.RXB,
			TXB: stats.TXB,
			RXS: stats.RXS,
			TXS: stats.TXS,
		},
	}

	pathByIface := make(map[string][]transport.PathTableEntry)
	var hopSum int
	var hopsList []int
	for _, p := range paths {
		h := int(p.Hops)
		hopSum += h
		hopsList = append(hopsList, h)
		if h >= opts.HighHopThreshold {
			rep.PathStats.HighHop++
		}
		if h >= opts.VeryHighHopThreshold {
			rep.PathStats.VeryHigh++
		}
		if h > rep.PathStats.MaxHops {
			rep.PathStats.MaxHops = h
		}
		if p.Interface != "" {
			pathByIface[p.Interface] = append(pathByIface[p.Interface], p)
		}
	}
	rep.PathStats.Total = len(paths)
	if len(paths) > 0 {
		rep.PathStats.AvgHops = float64(hopSum) / float64(len(paths))
		sort.Ints(hopsList)
		rep.PathStats.MedianHops = hopsList[len(hopsList)/2]
	}

	ifaceRows := make([]SlowIfaceRow, 0, len(stats.Interfaces))
	for _, st := range stats.Interfaces {
		if hideInterface(st.Name, opts.ShowAll) {
			continue
		}
		if opts.NameFilter != "" && !strings.Contains(strings.ToLower(st.Name), strings.ToLower(opts.NameFilter)) {
			continue
		}
		row := scoreInterface(st, pathByIface[st.Name], opts)
		ifaceRows = append(ifaceRows, row)
	}
	sort.SliceStable(ifaceRows, func(i, j int) bool {
		if ifaceRows[i].Score == ifaceRows[j].Score {
			return ifaceRows[i].Name < ifaceRows[j].Name
		}
		return ifaceRows[i].Score > ifaceRows[j].Score
	})
	if len(ifaceRows) > opts.TopN {
		ifaceRows = ifaceRows[:opts.TopN]
	}
	rep.Interfaces = ifaceRows

	hotspots := make([]PathEgressHotspot, 0, len(pathByIface))
	for name, list := range pathByIface {
		hs := PathEgressHotspot{Interface: name, Paths: len(list)}
		sum := 0
		for _, p := range list {
			h := int(p.Hops)
			sum += h
			if h > hs.MaxHops {
				hs.MaxHops = h
			}
			if h >= opts.HighHopThreshold {
				hs.HighHop++
			}
		}
		if len(list) > 0 {
			hs.AvgHops = float64(sum) / float64(len(list))
		}
		hotspots = append(hotspots, hs)
	}
	sort.SliceStable(hotspots, func(i, j int) bool {
		if hotspots[i].Paths == hotspots[j].Paths {
			return hotspots[i].AvgHops > hotspots[j].AvgHops
		}
		return hotspots[i].Paths > hotspots[j].Paths
	})
	if len(hotspots) > 8 {
		hotspots = hotspots[:8]
	}
	rep.EgressHotspots = hotspots

	rep.Findings = buildFindings(rep, opts)
	rep.Recommendations = buildRecommendations(rep, opts)
	return rep
}

func scoreInterface(st transport.InterfaceStat, paths []transport.PathTableEntry, opts SlowAnalyzeOptions) SlowIfaceRow {
	load := math.Max(st.RXS, st.TXS)
	util := 0.0
	if st.Bitrate >= opts.MinBitrateForUtil {
		util = (load / float64(st.Bitrate)) * 100
	}
	row := SlowIfaceRow{
		Name:               st.Name,
		Type:               st.Type,
		Online:             st.Status,
		Bitrate:            st.Bitrate,
		RXS:                st.RXS,
		TXS:                st.TXS,
		RXB:                st.RXB,
		TXB:                st.TXB,
		UtilPct:            util,
		HeldAnnounces:      st.HeldAnnounces,
		BurstActive:        st.BurstActive,
		PRBurstActive:      st.PRBurstActive,
		AnnounceHz:         st.IncomingAnnounceFrequency + st.OutgoingAnnounceFrequency,
		PRHz:               st.IncomingPRFrequency + st.OutgoingPRFrequency,
		RTTMs:              st.RTTMs,
		BandwidthAvailable: st.BandwidthAvailable,
		PathCount:          len(paths),
		IFACFail:           st.IFACFail,
		HMACFail:           st.HMACFail,
		AnnounceSigFail:    st.AnnounceSigFail,
		UnpackFail:         st.UnpackFail,
		IntegrityFailRate:  st.IntegrityFailRate,
		IntegritySamples60: st.IntegritySamples60,
		StaleCloses:        st.StaleCloses,
		KeepaliveTimeout:   st.KeepaliveTimeout,
		ProofFail:          st.ProofFail,
	}

	var flags []string
	var reasons []string
	score := 0.0

	if !st.Status {
		flags = append(flags, "offline")
		reasons = append(reasons, "interface offline")
		score += 40
		if len(paths) > 0 {
			reasons = append(reasons, fmt.Sprintf("%d paths still egress here", len(paths)))
			score += 30
		}
	}
	if st.BurstActive {
		flags = append(flags, "announce-burst")
		reasons = append(reasons, "announce ingress burst limiter active")
		score += 35
	}
	if st.PRBurstActive {
		flags = append(flags, "pr-burst")
		reasons = append(reasons, "path-request burst limiter active")
		score += 25
	}
	if st.HeldAnnounces > 0 {
		flags = append(flags, fmt.Sprintf("held=%d", st.HeldAnnounces))
		reasons = append(reasons, fmt.Sprintf("%d announces held (ingress congestion)", st.HeldAnnounces))
		score += math.Min(40, float64(st.HeldAnnounces)*2)
	}
	if st.Bitrate > 0 && st.Bitrate < 500_000 && load > float64(st.Bitrate)*0.3 {
		flags = append(flags, "low-cap")
		reasons = append(reasons, fmt.Sprintf("configured bitrate only %s", prettyBitrateCap(st.Bitrate)))
		score += 20
	}
	if util >= opts.UtilCritPct {
		flags = append(flags, "saturated")
		reasons = append(reasons, fmt.Sprintf("load %.0f%% of bitrate cap", util))
		score += 50
	} else if util >= opts.UtilWarnPct {
		flags = append(flags, "busy")
		reasons = append(reasons, fmt.Sprintf("load %.0f%% of bitrate cap", util))
		score += 30
	}
	if st.BandwidthAvailable != nil && !*st.BandwidthAvailable {
		flags = append(flags, "bw-gate")
		reasons = append(reasons, "bandwidth gate closed (announce/path forwarding throttled)")
		score += 35
	}
	if st.RTTMs != nil && *st.RTTMs >= 200 {
		flags = append(flags, fmt.Sprintf("rtt=%.0fms", *st.RTTMs))
		reasons = append(reasons, fmt.Sprintf("socket RTT %.0f ms", *st.RTTMs))
		if *st.RTTMs >= 500 {
			score += 25
		} else {
			score += 12
		}
	}
	if len(paths) >= 50 {
		flags = append(flags, fmt.Sprintf("paths=%d", len(paths)))
		reasons = append(reasons, fmt.Sprintf("egress for %d known paths", len(paths)))
		score += math.Min(25, float64(len(paths))/20)
	}
	if row.AnnounceHz > 5 {
		flags = append(flags, "announce-storm")
		reasons = append(reasons, fmt.Sprintf("%.1f announces/s", row.AnnounceHz))
		score += 15
	}

	integritySamples := st.IntegritySamples60
	if integritySamples >= opts.MinIntegritySamples && st.IntegrityFailRate >= opts.IntegrityWarnRate {
		flags = append(flags, "integrity")
		reasons = append(reasons, fmt.Sprintf("integrity fail rate %.0f%%", st.IntegrityFailRate*100))
		if st.IntegrityFailRate >= opts.IntegrityCritRate {
			score += 45
		} else {
			score += 25
		}
		// Quiet low-bitrate links need more failures before sounding critical.
		if st.Bitrate > 0 && st.Bitrate < opts.MinBitrateForUtil && st.IntegrityFailRate < opts.IntegrityCritRate {
			score -= 10
		}
	}
	authFails := st.AnnounceSigFail + st.ProofFail
	if authFails >= opts.AuthFailWarn {
		flags = append(flags, "auth-pressure")
		reasons = append(reasons, fmt.Sprintf("%d auth rejects (announce/proof)", authFails))
		if authFails >= opts.AuthFailCrit {
			score += 40
		} else {
			score += 22
		}
	}
	staleScore := st.StaleCloses + st.KeepaliveTimeout
	if staleScore >= opts.StaleWarn {
		flags = append(flags, "link-degraded")
		reasons = append(reasons, fmt.Sprintf("%d stale/keepalive closes", staleScore))
		if staleScore >= opts.StaleCrit {
			score += 35
		} else {
			score += 18
		}
	}

	row.Flags = flags
	row.Score = score
	if len(reasons) == 0 {
		if st.Bitrate > 0 {
			row.Why = fmt.Sprintf("ok · cap %s · load %s", prettyBitrateCap(st.Bitrate), prettyBitrate(int64(load)))
		} else {
			row.Why = "ok · no bitrate cap reported"
		}
	} else {
		row.Why = strings.Join(reasons, ", ")
	}
	return row
}

func buildFindings(rep SlowReport, opts SlowAnalyzeOptions) []SlowFinding {
	var out []SlowFinding
	for _, row := range rep.Interfaces {
		if row.Score < 15 {
			continue
		}
		sev := SeverityWarn
		if row.Score >= 60 || !row.Online {
			sev = SeverityCritical
		}
		out = append(out, SlowFinding{
			Kind:     "interface",
			Name:     row.Name,
			Severity: sev,
			Score:    row.Score,
			Summary:  row.Why,
			Detail: fmt.Sprintf("cap=%s load_rx=%s load_tx=%s util=%.0f%% paths=%d",
				prettyBitrateCap(row.Bitrate), prettyBitrate(int64(row.RXS)), prettyBitrate(int64(row.TXS)),
				row.UtilPct, row.PathCount),
			Hints: interfaceHints(row),
		})
		out = append(out, healthFindingsForRow(row, opts)...)
	}
	if rep.PathStats.VeryHigh > 0 {
		out = append(out, SlowFinding{
			Kind:     "path",
			Name:     "high-hop paths",
			Severity: SeverityWarn,
			Score:    float64(rep.PathStats.VeryHigh),
			Summary: fmt.Sprintf("%d paths with hops >= %d (median %d, max %d)",
				rep.PathStats.VeryHigh, opts.VeryHighHopThreshold, rep.PathStats.MedianHops, rep.PathStats.MaxHops),
			Detail: fmt.Sprintf("%d paths hops>=%d of %d total",
				rep.PathStats.HighHop, opts.HighHopThreshold, rep.PathStats.Total),
			Hints: []string{
				"Resource transfers over many hops accumulate per-hop delay and retries",
				"Prefer destinations with shorter paths when possible",
			},
		})
	}
	for _, hs := range rep.EgressHotspots {
		if hs.Paths < 30 && hs.HighHop < 5 {
			continue
		}
		congested := false
		for _, row := range rep.Interfaces {
			if row.Name == hs.Interface && row.Score >= 30 {
				congested = true
				break
			}
		}
		sev := SeverityInfo
		score := float64(hs.Paths) / 10
		if congested {
			sev = SeverityCritical
			score += 40
		} else if hs.HighHop > 10 {
			sev = SeverityWarn
			score += 15
		}
		out = append(out, SlowFinding{
			Kind:     "path",
			Name:     "egress " + hs.Interface,
			Severity: sev,
			Score:    score,
			Summary: fmt.Sprintf("%d paths via this iface (avg hops %.1f, max %d, high-hop %d)",
				hs.Paths, hs.AvgHops, hs.MaxHops, hs.HighHop),
			Hints: []string{
				"Transfers to destinations on this egress share the same uplink budget",
			},
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Score == out[j].Score {
			return out[i].Name < out[j].Name
		}
		return out[i].Score > out[j].Score
	})
	return out
}

func healthFindingsForRow(row SlowIfaceRow, opts SlowAnalyzeOptions) []SlowFinding {
	var out []SlowFinding
	integritySamples := row.IntegritySamples60
	if integritySamples >= opts.MinIntegritySamples && row.IntegrityFailRate >= opts.IntegrityWarnRate {
		sev := SeverityWarn
		score := 40.0
		if row.IntegrityFailRate >= opts.IntegrityCritRate {
			sev = SeverityCritical
			score = 70
		}
		out = append(out, SlowFinding{
			Kind:     "integrity_burst",
			Name:     row.Name,
			Severity: sev,
			Score:    score,
			Summary:  fmt.Sprintf("integrity fail rate %.0f%% on %s", row.IntegrityFailRate*100, row.Name),
			Detail: fmt.Sprintf("ifac=%d hmac=%d unpack=%d fail_rate=%.3f",
				row.IFACFail, row.HMACFail, row.UnpackFail, row.IntegrityFailRate),
			Hints: []string{
				"Check IFAC netname/netkey match on this interface",
				"On radio links elevated HMAC/IFAC fails can mean RF noise or bit flips",
			},
		})
	}
	authFails := row.AnnounceSigFail + row.ProofFail
	if authFails >= opts.AuthFailWarn {
		sev := SeverityWarn
		score := 35.0
		if authFails >= opts.AuthFailCrit {
			sev = SeverityCritical
			score = 65
		}
		out = append(out, SlowFinding{
			Kind:     "auth_pressure",
			Name:     row.Name,
			Severity: sev,
			Score:    score,
			Summary:  fmt.Sprintf("%d announce/proof rejects on %s", authFails, row.Name),
			Detail:   fmt.Sprintf("announce_sig=%d proof=%d", row.AnnounceSigFail, row.ProofFail),
			Hints: []string{
				"Forged or corrupt announces and proofs are dropped. Review who can reach this interface",
			},
		})
	}
	staleScore := row.StaleCloses + row.KeepaliveTimeout
	if staleScore >= opts.StaleWarn {
		sev := SeverityWarn
		score := 30.0
		if staleScore >= opts.StaleCrit {
			sev = SeverityCritical
			score = 55
		}
		out = append(out, SlowFinding{
			Kind:     "link_degraded",
			Name:     row.Name,
			Severity: sev,
			Score:    score,
			Summary:  fmt.Sprintf("%d stale or keepalive closes via %s", staleScore, row.Name),
			Detail:   fmt.Sprintf("stale_closes=%d keepalive_timeout=%d", row.StaleCloses, row.KeepaliveTimeout),
			Hints: []string{
				"Rising stale closes often means loss, extreme latency, or a peer that went away",
			},
		})
	}
	if row.BurstActive || row.HeldAnnounces > 0 || row.PRBurstActive {
		out = append(out, SlowFinding{
			Kind:     "ingress_pressure",
			Name:     row.Name,
			Severity: SeverityWarn,
			Score:    28,
			Summary:  fmt.Sprintf("ingress pressure on %s (held=%d burst=%v pr_burst=%v)", row.Name, row.HeldAnnounces, row.BurstActive, row.PRBurstActive),
			Hints: []string{
				"Ingress hold and burst limiters are protecting this node from announce floods",
			},
		})
	}
	return out
}

func interfaceHints(row SlowIfaceRow) []string {
	var hints []string
	if row.Bitrate > 0 && row.UtilPct >= 60 {
		hints = append(hints, "Raise interface bitrate in config, or add another uplink")
	}
	if row.BurstActive || row.HeldAnnounces > 0 {
		hints = append(hints, "Announce storm or slow peer: held announces delay path learning and can stall transfers")
	}
	if row.BandwidthAvailable != nil && !*row.BandwidthAvailable {
		hints = append(hints, "Bandwidth gate is closed: announce/path rebroadcasts are deferred until load drops")
	}
	if row.RTTMs != nil && *row.RTTMs >= 200 {
		hints = append(hints, "High socket RTT on this hop multiplies link establishment and resource round-trips")
	}
	if !row.Online {
		hints = append(hints, "Reconnect or disable this interface so paths can move to a live egress")
	}
	if row.IntegrityFailRate >= 0.05 {
		hints = append(hints, "Integrity failures are elevated. Verify IFAC keys and physical link quality")
	}
	if row.AnnounceSigFail+row.ProofFail >= 5 {
		hints = append(hints, "Auth rejects are elevated. Unexpected peers may be probing this segment")
	}
	if row.StaleCloses+row.KeepaliveTimeout >= 3 {
		hints = append(hints, "Links are going stale. Check latency, loss, and peer uptime on this path")
	}
	return hints
}

func buildRecommendations(rep SlowReport, opts SlowAnalyzeOptions) []string {
	seen := make(map[string]struct{})
	var out []string
	add := func(s string) {
		if _, ok := seen[s]; ok {
			return
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	for _, f := range rep.Findings {
		for _, h := range f.Hints {
			add(h)
		}
	}
	if rep.PathStats.MedianHops >= opts.HighHopThreshold {
		add(fmt.Sprintf("Typical path length is %d hops: expect multi-second resource RTTs even on healthy links",
			rep.PathStats.MedianHops))
	}
	if len(rep.Interfaces) > 0 && rep.Interfaces[0].Score >= 40 {
		add(fmt.Sprintf("Primary bottleneck candidate: %s", rep.Interfaces[0].Name))
	}
	if len(out) == 0 {
		add("No strong local bottlenecks from interface/path stats. If transfers are still slow, probe the destination RTT and check remote peer load")
	}
	return out
}

func prettyBitrate(bps int64) string {
	if bps < 0 {
		return "n/a"
	}
	if bps == 0 {
		return "0 bps"
	}
	return SizeString(float64(bps), "bps")
}

func prettyBitrateCap(bps int64) string {
	if bps <= 0 {
		return "n/a"
	}
	return SizeString(float64(bps), "bps")
}

// WriteSlowJSON emits the report as JSON.
func WriteSlowJSON(w io.Writer, rep SlowReport) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(rep)
}

// WriteSlowHuman prints a readable bottleneck report.
func WriteSlowHuman(w io.Writer, rep SlowReport) error {
	title := "rgoslow - transfer bottleneck report"
	fmt.Fprintln(w, term.BoldW(w, title))
	fmt.Fprintf(w, "RPC %s", rep.RPCAddr)
	if rep.TransportID != "" {
		fmt.Fprintf(w, "  transport %s", rep.TransportID)
	}
	if rep.TransportUptime > 0 {
		fmt.Fprintf(w, "  uptime %s", formatUptime(rep.TransportUptime))
	}
	if rep.LinkCount != nil {
		fmt.Fprintf(w, "  links %d", *rep.LinkCount)
	}
	fmt.Fprintln(w)
	fmt.Fprintf(w, "Traffic  rx %s (%s)  tx %s (%s)\n",
		SizeString(float64(rep.Totals.RXB), "B"), prettyBitrate(int64(rep.Totals.RXS)),
		SizeString(float64(rep.Totals.TXB), "B"), prettyBitrate(int64(rep.Totals.TXS)))
	fmt.Fprintf(w, "Paths    %d total  median %d hops  high-hop(>=%d) %d  max %d\n",
		rep.PathStats.Total, rep.PathStats.MedianHops, rep.HighHopAt, rep.PathStats.HighHop, rep.PathStats.MaxHops)
	fmt.Fprintln(w)

	crit, warn := 0, 0
	for _, f := range rep.Findings {
		switch f.Severity {
		case SeverityCritical:
			crit++
		case SeverityWarn:
			warn++
		}
	}
	fmt.Fprintln(w, term.BoldW(w, "SUMMARY"))
	sumLine := fmt.Sprintf("  %d critical  %d warnings  %d findings", crit, warn, len(rep.Findings))
	if crit > 0 {
		fmt.Fprintln(w, term.RedW(w, sumLine))
	} else if warn > 0 {
		fmt.Fprintln(w, term.YellowW(w, sumLine))
	} else {
		fmt.Fprintln(w, term.GreenW(w, sumLine+"  (no strong local bottlenecks)"))
	}
	fmt.Fprintln(w)

	if rep.Destination != nil {
		fmt.Fprintln(w, term.BoldW(w, "DESTINATION"))
		d := rep.Destination
		if !d.HasPath {
			fmt.Fprintf(w, "  %s  %s\n", d.Hash, term.RedW(w, "no path"))
		} else {
			fmt.Fprintf(w, "  %s", d.Hash)
			if d.Hops > 0 {
				fmt.Fprintf(w, "  %d hops", d.Hops)
			}
			if d.Via != "" {
				fmt.Fprintf(w, " via %s", d.Via)
			}
			if d.Interface != "" {
				fmt.Fprintf(w, " on %s", d.Interface)
			}
			fmt.Fprintln(w)
			if d.FirstHopTimeoutS > 0 {
				fmt.Fprintf(w, "  first-hop timeout %.1fs", d.FirstHopTimeoutS)
				if d.ExpiresInS > 0 {
					fmt.Fprintf(w, "  path expires in %.0fs", d.ExpiresInS)
				}
				fmt.Fprintln(w)
			}
		}
		fmt.Fprintln(w)
	}

	fmt.Fprintln(w, term.BoldW(w, "INTERFACES (ranked by constraint)"))
	if len(rep.Interfaces) == 0 {
		fmt.Fprintln(w, "  (none)")
	} else {
		fmt.Fprintf(w, "  %-28s %10s %10s %6s  %s\n", "NAME", "CAP", "LOAD", "UTIL", "WHY")
		for i, row := range rep.Interfaces {
			load := math.Max(row.RXS, row.TXS)
			name := row.Name
			if len(name) > 28 {
				name = name[:25] + "..."
			}
			line := fmt.Sprintf("  %-28s %10s %10s %5.0f%%  %s",
				name, prettyBitrateCap(row.Bitrate), prettyBitrate(int64(load)), row.UtilPct, row.Why)
			switch {
			case row.Score >= 60 || !row.Online:
				fmt.Fprintln(w, term.RedW(w, line))
			case row.Score >= 25:
				fmt.Fprintln(w, term.YellowW(w, line))
			default:
				if i < 5 {
					fmt.Fprintln(w, line)
				}
			}
		}
	}
	fmt.Fprintln(w)

	if len(rep.EgressHotspots) > 0 {
		fmt.Fprintln(w, term.BoldW(w, "PATH EGRESS HOTSPOTS"))
		fmt.Fprintf(w, "  %-28s %6s %8s %7s %8s\n", "INTERFACE", "PATHS", "AVG_HOP", "MAX", "HIGH")
		for _, hs := range rep.EgressHotspots {
			name := hs.Interface
			if len(name) > 28 {
				name = name[:25] + "..."
			}
			fmt.Fprintf(w, "  %-28s %6d %8.1f %7d %8d\n",
				name, hs.Paths, hs.AvgHops, hs.MaxHops, hs.HighHop)
		}
		fmt.Fprintln(w)
	}

	if len(rep.Findings) > 0 {
		fmt.Fprintln(w, term.BoldW(w, "FINDINGS"))
		for _, f := range rep.Findings {
			prefix := "·"
			line := fmt.Sprintf("  %s [%s] %s: %s", prefix, f.Severity, f.Name, f.Summary)
			switch f.Severity {
			case SeverityCritical:
				fmt.Fprintln(w, term.RedW(w, line))
			case SeverityWarn:
				fmt.Fprintln(w, term.YellowW(w, line))
			default:
				fmt.Fprintln(w, line)
			}
			if f.Detail != "" {
				fmt.Fprintf(w, "      %s\n", f.Detail)
			}
		}
		fmt.Fprintln(w)
	}

	fmt.Fprintln(w, term.BoldW(w, "RECOMMENDATIONS"))
	for _, r := range rep.Recommendations {
		fmt.Fprintf(w, "  · %s\n", r)
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, term.CyanW(w, "Tip: rgoslow -dest <hash> focuses one destination; -paths adds hop/egress analysis; -json for scripting"))
	fmt.Fprintln(w, term.CyanW(w, "Works against Go (reticulum-go) and Python (rnsd) shared instances via RPC"))
	return nil
}

func formatUptime(sec float64) string {
	if sec < 60 {
		return fmt.Sprintf("%.0fs", sec)
	}
	if sec < 3600 {
		return fmt.Sprintf("%.0fm", sec/60)
	}
	if sec < 86400 {
		return fmt.Sprintf("%.1fh", sec/3600)
	}
	return fmt.Sprintf("%.1fd", sec/86400)
}
