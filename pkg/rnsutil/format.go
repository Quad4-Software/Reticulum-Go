// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package rnsutil

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"

	"quad4/reticulum-go/pkg/discovery"
	"quad4/reticulum-go/pkg/protect"
	"quad4/reticulum-go/pkg/term"
	"quad4/reticulum-go/pkg/transport"
)

// PrettyHex formats a hash as <aabb...ccdd>.
func PrettyHex(b []byte) string {
	if len(b) == 0 {
		return "<>"
	}
	h := hex.EncodeToString(b)
	if len(h) <= 12 {
		return "<" + h + ">"
	}
	return "<" + h[:6] + "..." + h[len(h)-6:] + ">"
}

// SizeString formats a byte count with SI prefixes.
func SizeString(num float64, suffix string) string {
	units := [...]string{"", "K", "M", "G", "T", "P", "E", "Z"}
	for _, unit := range units {
		if abs64(num) < 1000 {
			if unit == "" {
				return fmt.Sprintf("%.0f %s%s", num, unit, suffix)
			}
			return fmt.Sprintf("%.2f %s%s", num, unit, suffix)
		}
		num /= 1000
	}
	return fmt.Sprintf("%.2f Y%s", num, suffix)
}

func abs64(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

// ModeName maps interface mode bytes to display labels.
func ModeName(mode byte) string {
	switch mode {
	case 0x03:
		return "Access Point"
	case 0x02:
		return "Point-to-Point"
	case 0x04:
		return "Roaming"
	case 0x05:
		return "Boundary"
	case 0x06:
		return "Gateway"
	case 0x07:
		return "Internal"
	default:
		return "Full"
	}
}

// StatusOptions controls human status rendering.
type StatusOptions struct {
	NameFilter     string
	SortBy         string
	SortAsc        bool
	ShowAll        bool
	AnnounceStats  bool
	PRStats        bool
	ShowBlockedIPs bool
	QueueStats     bool
	TrafficTotals  bool
	BurstFilter    bool
	ShowPPS        bool
}

// SortInterfaceStats sorts interfaces in place by SortBy.
func SortInterfaceStats(stats *transport.InterfaceStatsResponse, sortBy string, asc bool) {
	if stats == nil || sortBy == "" {
		return
	}
	less := func(i, j int) bool { return false }
	switch strings.ToLower(sortBy) {
	case "rate", "bitrate":
		less = func(i, j int) bool { return stats.Interfaces[i].Bitrate < stats.Interfaces[j].Bitrate }
	case "rx":
		less = func(i, j int) bool { return stats.Interfaces[i].RXB < stats.Interfaces[j].RXB }
	case "tx":
		less = func(i, j int) bool { return stats.Interfaces[i].TXB < stats.Interfaces[j].TXB }
	case "rxs":
		less = func(i, j int) bool { return stats.Interfaces[i].RXS < stats.Interfaces[j].RXS }
	case "txs":
		less = func(i, j int) bool { return stats.Interfaces[i].TXS < stats.Interfaces[j].TXS }
	case "traffic":
		less = func(i, j int) bool {
			return stats.Interfaces[i].RXB+stats.Interfaces[i].TXB < stats.Interfaces[j].RXB+stats.Interfaces[j].TXB
		}
	case "announces", "announce":
		less = func(i, j int) bool {
			a := stats.Interfaces[i].IncomingAnnounceFrequency + stats.Interfaces[i].OutgoingAnnounceFrequency
			b := stats.Interfaces[j].IncomingAnnounceFrequency + stats.Interfaces[j].OutgoingAnnounceFrequency
			return a < b
		}
	case "arx":
		less = func(i, j int) bool {
			return stats.Interfaces[i].IncomingAnnounceFrequency < stats.Interfaces[j].IncomingAnnounceFrequency
		}
	case "atx":
		less = func(i, j int) bool {
			return stats.Interfaces[i].OutgoingAnnounceFrequency < stats.Interfaces[j].OutgoingAnnounceFrequency
		}
	case "prx":
		less = func(i, j int) bool {
			return stats.Interfaces[i].IncomingPRFrequency < stats.Interfaces[j].IncomingPRFrequency
		}
	case "ptx":
		less = func(i, j int) bool {
			return stats.Interfaces[i].OutgoingPRFrequency < stats.Interfaces[j].OutgoingPRFrequency
		}
	case "held":
		less = func(i, j int) bool {
			return stats.Interfaces[i].HeldAnnounces < stats.Interfaces[j].HeldAnnounces
		}
	case "queue":
		less = func(i, j int) bool {
			return stats.Interfaces[i].AnnounceQueue < stats.Interfaces[j].AnnounceQueue
		}
	case "pvs":
		less = func(i, j int) bool {
			return stats.Interfaces[i].ProtocolViolations < stats.Interfaces[j].ProtocolViolations
		}
	case "ivs":
		less = func(i, j int) bool {
			return stats.Interfaces[i].IFACViolations < stats.Interfaces[j].IFACViolations
		}
	case "flt":
		less = func(i, j int) bool {
			return stats.Interfaces[i].PacketFilterHits < stats.Interfaces[j].PacketFilterHits
		}
	case "arxc":
		less = func(i, j int) bool {
			return stats.Interfaces[i].ARXC < stats.Interfaces[j].ARXC
		}
	case "atxc":
		less = func(i, j int) bool {
			return stats.Interfaces[i].ATXC < stats.Interfaces[j].ATXC
		}
	case "prxc":
		less = func(i, j int) bool {
			return stats.Interfaces[i].PRXC < stats.Interfaces[j].PRXC
		}
	case "ptxc":
		less = func(i, j int) bool {
			return stats.Interfaces[i].PTXC < stats.Interfaces[j].PTXC
		}
	case "gravity", "g":
		less = func(i, j int) bool {
			return stats.Interfaces[i].Gravity < stats.Interfaces[j].Gravity
		}
	default:
		return
	}
	sort.SliceStable(stats.Interfaces, func(i, j int) bool {
		if asc {
			return less(i, j)
		}
		return less(j, i)
	})
}

func hideInterface(name string, showAll bool) bool {
	if showAll {
		return false
	}
	prefixes := []string{
		"LocalInterface[",
		"TCPInterface[Client",
		"BackboneInterface[Client on",
		"AutoInterfacePeer[",
		"WeaveInterfacePeer[",
		"I2PInterfacePeer[Connected peer",
	}
	for _, p := range prefixes {
		if strings.HasPrefix(name, p) {
			return true
		}
	}
	return false
}

// WriteStatusHuman writes a human-readable status report including announce rates.
func WriteStatusHuman(w io.Writer, stats transport.InterfaceStatsResponse, linkCount *int, activeLinkCount *int, opts StatusOptions) error {
	filter := strings.ToLower(opts.NameFilter)
	for i := range stats.Interfaces {
		st := &stats.Interfaces[i]
		if hideInterface(st.Name, opts.ShowAll) {
			continue
		}
		if opts.BurstFilter && !(st.BurstActive || st.PRBurstActive) {
			if filter == "" {
				continue
			}
		}
		if filter != "" && !strings.Contains(strings.ToLower(st.Name), filter) {
			continue
		}
		state := "Down"
		if st.Status {
			state = "Up"
		}
		if st.Status {
			state = term.GreenW(w, state)
		} else {
			state = term.RedW(w, state)
		}
		if _, err := fmt.Fprintf(w, "\n%s\n  Status    : %s\n  Mode      : %s\n  RX        : %s\n  TX        : %s\n",
			st.Name, state, ModeName(st.Mode),
			SizeString(float64(st.RXB), "B"),
			SizeString(float64(st.TXB), "B"),
		); err != nil {
			return err
		}
		if st.RXS > 0 || st.TXS > 0 {
			if _, err := fmt.Fprintf(w, "  Rate      : ↓%s ↑%s\n",
				SizeString(st.RXS, "b")+"/s",
				SizeString(st.TXS, "b")+"/s",
			); err != nil {
				return err
			}
		}
		if st.Bitrate > 0 {
			if _, err := fmt.Fprintf(w, "  Bitrate   : %s\n", SizeString(float64(st.Bitrate), "b")+"/s"); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintf(w, "  Announces : ↓%.2f/s ↑%.2f/s\n  Path reqs : ↓%.2f/s ↑%.2f/s\n  Held      : %d\n  Queue     : %d\n",
			st.IncomingAnnounceFrequency, st.OutgoingAnnounceFrequency,
			st.IncomingPRFrequency, st.OutgoingPRFrequency,
			st.HeldAnnounces,
			st.AnnounceQueue,
		); err != nil {
			return err
		}
		if st.BurstActive || st.PRBurstActive {
			if _, err := fmt.Fprintf(w, "  Burst     : announce=%v pathreq=%v\n", st.BurstActive, st.PRBurstActive); err != nil {
				return err
			}
		}
		integrityFails := st.IFACFail + st.HMACFail + st.UnpackFail + st.PaddingFail + st.AnnounceSigFail
		if integrityFails > 0 || st.StaleCloses > 0 || st.IntegrityFailRate > 0 {
			if _, err := fmt.Fprintf(w, "  Integrity : fail_rate=%.2f ifac=%d hmac=%d unpack=%d announce_sig=%d stale_closes=%d\n",
				st.IntegrityFailRate, st.IFACFail, st.HMACFail, st.UnpackFail, st.AnnounceSigFail, st.StaleCloses,
			); err != nil {
				return err
			}
		}
		healthBits := st.BlackholeHit + st.PathReqDup + st.PathReqNoCache + st.PathRespSuppressed +
			st.ResourceStall + st.LinkStaleClose + st.KeepaliveTimeout + st.AnnounceDup
		if healthBits > 0 {
			if _, err := fmt.Fprintf(w, "  Health    : blackhole=%d path_dup=%d path_nocache=%d path_suppress=%d resource_stall=%d link_stale=%d keepalive=%d announce_dup=%d\n",
				st.BlackholeHit, st.PathReqDup, st.PathReqNoCache, st.PathRespSuppressed,
				st.ResourceStall, st.LinkStaleClose, st.KeepaliveTimeout, st.AnnounceDup,
			); err != nil {
				return err
			}
		}
		if st.Clients != nil {
			if _, err := fmt.Fprintf(w, "  Clients   : %d\n", *st.Clients); err != nil {
				return err
			}
			if st.BlockedIPs != nil && *st.BlockedIPs > 0 {
				if _, err := fmt.Fprintf(w, "  Blocked   : %d IPs\n", *st.BlockedIPs); err != nil {
					return err
				}
			}
			if opts.ShowBlockedIPs && len(st.BlockedIPList) > 0 {
				for _, bip := range st.BlockedIPList {
					if _, err := fmt.Fprintf(w, "              %s\n", bip); err != nil {
						return err
					}
				}
			}
		}
		if st.ProtocolViolations > 0 || st.IFACViolations > 0 {
			pv := ""
			if st.ProtocolViolations > 0 {
				pv = fmt.Sprintf("%d protocol", st.ProtocolViolations)
			}
			iv := ""
			if st.IFACViolations > 0 {
				iv = fmt.Sprintf("%d IFAC", st.IFACViolations)
			}
			sep := ""
			if pv != "" && iv != "" {
				sep = ", "
			}
			if _, err := fmt.Fprintf(w, "  Violatns. : %s%s%s\n", pv, sep, iv); err != nil {
				return err
			}
		}
		if st.PacketFilterHits > 0 {
			if _, err := fmt.Fprintf(w, "  Flt. Hits : %d\n", st.PacketFilterHits); err != nil {
				return err
			}
		}
		if opts.PRStats {
			if _, err := fmt.Fprintf(w, "  Path Rqs. : ↓%s ↑%s (%.2f/s ↓ %.2f/s ↑)\n",
				SizeString(float64(st.PRXB), "B"),
				SizeString(float64(st.PTXB), "B"),
				st.IncomingPRFrequency,
				st.OutgoingPRFrequency,
			); err != nil {
				return err
			}
		}
		if opts.AnnounceStats {
			if _, err := fmt.Fprintf(w, "  Announces : ↓%s ↑%s (%.2f/s ↓ %.2f/s ↑)\n",
				SizeString(float64(st.ARXB), "B"),
				SizeString(float64(st.ATXB), "B"),
				st.IncomingAnnounceFrequency,
				st.OutgoingAnnounceFrequency,
			); err != nil {
				return err
			}
		}
	}
	if len(stats.TransportID) > 0 {
		if _, err := fmt.Fprintf(w, "\nTransport ID : %s\n", PrettyHex(stats.TransportID)); err != nil {
			return err
		}
	}
	if stats.TransportUptime > 0 {
		if _, err := fmt.Fprintf(w, "Uptime       : %s\n", formatSeconds(stats.TransportUptime)); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(w, "Traffic      : RX %s  TX %s\n",
		SizeString(float64(stats.RXB), "B"),
		SizeString(float64(stats.TXB), "B"),
	); err != nil {
		return err
	}
	if stats.RXS > 0 || stats.TXS > 0 {
		if _, err := fmt.Fprintf(w, "Throughput   : ↓%s ↑%s\n",
			SizeString(stats.RXS, "b")+"/s",
			SizeString(stats.TXS, "b")+"/s",
		); err != nil {
			return err
		}
	}
	if linkCount != nil {
		line := fmt.Sprintf("Links        : %d", *linkCount)
		if activeLinkCount != nil && *activeLinkCount > 0 {
			line += fmt.Sprintf(" (%d active)", *activeLinkCount)
		}
		if _, err := fmt.Fprintf(w, "%s\n", line); err != nil {
			return err
		}
	}
	if opts.TrafficTotals {
		rxLine := fmt.Sprintf("↓%s  %s/s",
			SizeString(float64(stats.RXB), "B"),
			SizeString(stats.RXS, "b"),
		)
		txLine := fmt.Sprintf("↑%s  %s/s",
			SizeString(float64(stats.TXB), "B"),
			SizeString(stats.TXS, "b"),
		)
		if opts.ShowPPS {
			rxLine += ", " + SizeString(stats.RXPPS, "pps")
			txLine += ", " + SizeString(stats.TXPPS, "pps")
		}
		if _, err := fmt.Fprintf(w, "\n Totals       : %s\n                %s\n", txLine, rxLine); err != nil {
			return err
		}
		if opts.AnnounceStats && (stats.ARXB > 0 || stats.ATXB > 0) {
			if _, err := fmt.Fprintf(w, " Announces    : ↓%s ↑%s\n",
				SizeString(float64(stats.ARXB), "B"),
				SizeString(float64(stats.ATXB), "B"),
			); err != nil {
				return err
			}
		}
		if opts.PRStats && (stats.PRXB > 0 || stats.PTXB > 0) {
			if _, err := fmt.Fprintf(w, " Path Rqs.    : ↓%s ↑%s\n",
				SizeString(float64(stats.PRXB), "B"),
				SizeString(float64(stats.PTXB), "B"),
			); err != nil {
				return err
			}
		}
	}
	if opts.QueueStats {
		tqdp := ""
		if stats.RXQTD > 0 {
			tqdp = fmt.Sprintf(", %d dropped", stats.RXQTD)
		}
		if _, err := fmt.Fprintf(w, "\n Qu. Pressure : %.1f%% total, %d pkts%s\n",
			stats.TQPressure*100, stats.RXQT, tqdp,
		); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "                %.1f%% data, %d pkts\n",
			stats.DQPressure*100, stats.RXQD,
		); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "                %.1f%% announce, %d pkts\n",
			stats.AQPressure*100, stats.RXQA,
		); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "                %.1f%% path request, %d pkts\n",
			stats.PQPressure*100, stats.RXQP,
		); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "                %.1f%% ingress limiter, %d pkts\n",
			stats.ILQPressure*100, stats.RXQIL,
		); err != nil {
			return err
		}
	}
	return nil
}

func formatSeconds(s float64) string {
	if s < 60 {
		return strconv.FormatFloat(s, 'f', 1, 64) + "s"
	}
	if s < 3600 {
		return strconv.FormatFloat(s/60, 'f', 1, 64) + "m"
	}
	if s < 86400 {
		return strconv.FormatFloat(s/3600, 'f', 1, 64) + "h"
	}
	return strconv.FormatFloat(s/86400, 'f', 1, 64) + "d"
}

// WriteStatusJSON writes interface stats as JSON with bytes hex-encoded.
func WriteStatusJSON(w io.Writer, stats transport.InterfaceStatsResponse) error {
	type ifaceJSON struct {
		Name                      string   `json:"name"`
		ShortName                 string   `json:"short_name"`
		Hash                      string   `json:"hash,omitempty"`
		Type                      string   `json:"type"`
		RXB                       uint64   `json:"rxb"`
		TXB                       uint64   `json:"txb"`
		RXS                       float64  `json:"rxs"`
		TXS                       float64  `json:"txs"`
		Status                    bool     `json:"status"`
		Mode                      byte     `json:"mode"`
		Clients                   *int     `json:"clients"`
		BlockedIPs                *int     `json:"blocked_ips,omitempty"`
		BlockedIPList             []string `json:"blocked_ip_list,omitempty"`
		Bitrate                   int64    `json:"bitrate"`
		IncomingAnnounceFrequency float64  `json:"incoming_announce_frequency"`
		OutgoingAnnounceFrequency float64  `json:"outgoing_announce_frequency"`
		IncomingPRFrequency       float64  `json:"incoming_pr_frequency"`
		OutgoingPRFrequency       float64  `json:"outgoing_pr_frequency"`
		HeldAnnounces             int      `json:"held_announces"`
		AnnounceQueue             int      `json:"announce_queue"`
		BurstActive               bool     `json:"burst_active"`
		PRBurstActive             bool     `json:"pr_burst_active"`
		IFACFail                  uint64   `json:"ifac_fail"`
		HMACFail                  uint64   `json:"hmac_fail"`
		AnnounceSigFail           uint64   `json:"announce_sig_fail"`
		UnpackFail                uint64   `json:"unpack_fail"`
		IntegrityFailRate         float64  `json:"integrity_fail_rate"`
		StaleCloses               uint64   `json:"stale_closes"`
		KeepaliveTimeout          uint64   `json:"keepalive_timeout"`
		ProtocolViolations        uint64   `json:"protocol_violations"`
		IFACViolations            uint64   `json:"ifac_violations"`
		PacketFilterHits          uint64   `json:"packet_filter_hits"`
		I2PB32                    *string  `json:"i2p_b32,omitempty"`
		I2PConnectable            *bool    `json:"i2p_connectable,omitempty"`
		Tunnel                    *string  `json:"tunnelstate,omitempty"`
		I2PLastError              *string  `json:"i2p_last_error,omitempty"`
	}
	out := struct {
		Interfaces      []ifaceJSON      `json:"interfaces"`
		RXB             uint64           `json:"rxb"`
		TXB             uint64           `json:"txb"`
		RXS             float64          `json:"rxs"`
		TXS             float64          `json:"txs"`
		RXPPS           float64          `json:"rxpps"`
		TXPPS           float64          `json:"txpps"`
		RXQT            int              `json:"rxqt"`
		RXQD            int              `json:"rxqd"`
		RXQA            int              `json:"rxqa"`
		RXQP            int              `json:"rxqp"`
		RXQIL           int              `json:"rxqil"`
		RXQTD           int              `json:"rxqtd"`
		TQPressure      float64          `json:"tqpressure"`
		DQPressure      float64          `json:"dqpressure"`
		AQPressure      float64          `json:"aqpressure"`
		PQPressure      float64          `json:"pqpressure"`
		ILQPressure     float64          `json:"ilqpressure"`
		TransportID     string           `json:"transport_id"`
		TransportUptime float64          `json:"transport_uptime"`
		NetmonFlap      uint64           `json:"netmon_flap"`
		ActiveLinks     int              `json:"active_links"`
		Protect         protect.Snapshot `json:"protect"`
	}{
		Interfaces:      make([]ifaceJSON, 0, len(stats.Interfaces)),
		RXB:             stats.RXB,
		TXB:             stats.TXB,
		RXS:             stats.RXS,
		TXS:             stats.TXS,
		RXPPS:           stats.RXPPS,
		TXPPS:           stats.TXPPS,
		RXQT:            stats.RXQT,
		RXQD:            stats.RXQD,
		RXQA:            stats.RXQA,
		RXQP:            stats.RXQP,
		RXQIL:           stats.RXQIL,
		RXQTD:           stats.RXQTD,
		TQPressure:      stats.TQPressure,
		DQPressure:      stats.DQPressure,
		AQPressure:      stats.AQPressure,
		PQPressure:      stats.PQPressure,
		ILQPressure:     stats.ILQPressure,
		TransportID:     hex.EncodeToString(stats.TransportID),
		TransportUptime: stats.TransportUptime,
		NetmonFlap:      stats.NetmonFlap,
		ActiveLinks:     stats.ActiveLinks,
		Protect:         stats.Protect,
	}
	for i := range stats.Interfaces {
		st := &stats.Interfaces[i]
		out.Interfaces = append(out.Interfaces, ifaceJSON{
			Name:                      st.Name,
			ShortName:                 st.ShortName,
			Hash:                      hex.EncodeToString(st.Hash),
			Type:                      st.Type,
			RXB:                       st.RXB,
			TXB:                       st.TXB,
			RXS:                       st.RXS,
			TXS:                       st.TXS,
			Status:                    st.Status,
			Mode:                      st.Mode,
			Clients:                   st.Clients,
			BlockedIPs:                st.BlockedIPs,
			BlockedIPList:             st.BlockedIPList,
			Bitrate:                   st.Bitrate,
			IncomingAnnounceFrequency: st.IncomingAnnounceFrequency,
			OutgoingAnnounceFrequency: st.OutgoingAnnounceFrequency,
			IncomingPRFrequency:       st.IncomingPRFrequency,
			OutgoingPRFrequency:       st.OutgoingPRFrequency,
			HeldAnnounces:             st.HeldAnnounces,
			AnnounceQueue:             st.AnnounceQueue,
			BurstActive:               st.BurstActive,
			PRBurstActive:             st.PRBurstActive,
			IFACFail:                  st.IFACFail,
			HMACFail:                  st.HMACFail,
			AnnounceSigFail:           st.AnnounceSigFail,
			UnpackFail:                st.UnpackFail,
			IntegrityFailRate:         st.IntegrityFailRate,
			StaleCloses:               st.StaleCloses,
			KeepaliveTimeout:          st.KeepaliveTimeout,
			ProtocolViolations:        st.ProtocolViolations,
			IFACViolations:            st.IFACViolations,
			PacketFilterHits:          st.PacketFilterHits,
			I2PB32:                    st.I2PB32,
			I2PConnectable:            st.I2PConnectable,
			Tunnel:                    st.TunnelState,
			I2PLastError:              st.I2PLastError,
		})
	}
	enc := json.NewEncoder(w)
	return enc.Encode(out)
}

// WriteDiscoveredJSON emits discovered interface records as JSON.
func WriteDiscoveredJSON(w io.Writer, list []*discovery.DiscoveredInterface) error {
	type row struct {
		Type                string   `json:"type"`
		Name                string   `json:"name"`
		Status              string   `json:"status"`
		Transport           bool     `json:"transport"`
		ReachableOn         string   `json:"reachable_on,omitempty"`
		Port                int64    `json:"port,omitempty"`
		Hops                uint8    `json:"hops"`
		Value               int      `json:"value"`
		TransportID         string   `json:"transport_id"`
		NetworkID           string   `json:"network_id"`
		OperatorLXMFAddress string   `json:"operator_lxmf_address,omitempty"`
		Discovered          float64  `json:"discovered"`
		LastHeard           float64  `json:"last_heard"`
		HeardCount          int      `json:"heard_count"`
		ConfigEntry         string   `json:"config_entry,omitempty"`
		Latitude            *float64 `json:"latitude,omitempty"`
		Longitude           *float64 `json:"longitude,omitempty"`
		Height              *float64 `json:"height,omitempty"`
	}
	out := make([]row, 0, len(list))
	for _, rec := range list {
		if rec == nil {
			continue
		}
		r := row{
			Type:        rec.Type,
			Name:        rec.Name,
			Status:      rec.Status,
			Transport:   rec.Transport,
			ReachableOn: rec.ReachableOn,
			Port:        rec.Port,
			Hops:        rec.Hops,
			Value:       rec.Value,
			TransportID: hex.EncodeToString(rec.TransportID),
			NetworkID:   hex.EncodeToString(rec.NetworkID),
			Discovered:  rec.Discovered,
			LastHeard:   rec.LastHeard,
			HeardCount:  rec.HeardCount,
			ConfigEntry: rec.ConfigEntry,
			Latitude:    rec.Latitude,
			Longitude:   rec.Longitude,
			Height:      rec.Height,
		}
		if len(rec.OperatorLXMFAddress) > 0 {
			r.OperatorLXMFAddress = hex.EncodeToString(rec.OperatorLXMFAddress)
		}
		out = append(out, r)
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

// WriteDiscoveredHuman prints discovered interfaces (rnstatus -d / -D).
func WriteDiscoveredHuman(w io.Writer, list []*discovery.DiscoveredInterface, details bool) error {
	if len(list) == 0 {
		_, err := fmt.Fprintln(w)
		return err
	}
	now := time.Now()
	for idx, rec := range list {
		if rec == nil {
			continue
		}
		if idx > 0 {
			if details {
				if _, err := fmt.Fprintln(w, "\n==============================================="); err != nil {
					return err
				}
			} else if _, err := fmt.Fprintln(w); err != nil {
				return err
			}
		}
		status := rec.Status
		switch status {
		case "available":
			status = "Available"
		case "unknown":
			status = "Unknown"
		case "stale":
			status = "Stale"
		}
		transportStr := "Disabled"
		if rec.Transport {
			transportStr = "Enabled"
		}
		if details {
			if len(rec.NetworkID) > 0 && len(rec.TransportID) > 0 && !bytes.Equal(rec.TransportID, rec.NetworkID) {
				if _, err := fmt.Fprintf(w, "Network   ID : %x\n", rec.NetworkID); err != nil {
					return err
				}
			}
			if _, err := fmt.Fprintf(w, "Transport ID : %x\n", rec.TransportID); err != nil {
				return err
			}
			if len(rec.OperatorLXMFAddress) > 0 {
				if _, err := fmt.Fprintf(w, "Operator LXMF: %x\n", rec.OperatorLXMFAddress); err != nil {
					return err
				}
			}
		}
		if _, err := fmt.Fprintf(w, "Name         : %s\n", rec.Name); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "Type         : %s\n", rec.Type); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "Status       : %s\n", status); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "Transport    : %s\n", transportStr); err != nil {
			return err
		}
		if rec.ReachableOn != "" {
			if _, err := fmt.Fprintf(w, "Reachable on : %s\n", rec.ReachableOn); err != nil {
				return err
			}
		}
		if rec.HasPort {
			if _, err := fmt.Fprintf(w, "Port         : %d\n", rec.Port); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintf(w, "Hops         : %d\n", rec.Hops); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "Stamp value  : %d\n", rec.Value); err != nil {
			return err
		}
		if details {
			dago := now.Sub(time.Unix(int64(rec.Discovered), 0))
			hago := now.Sub(time.Unix(int64(rec.LastHeard), 0))
			if _, err := fmt.Fprintf(w, "Discovered   : %s ago\n", prettyDuration(dago)); err != nil {
				return err
			}
			if _, err := fmt.Fprintf(w, "Last heard   : %s ago\n", prettyDuration(hago)); err != nil {
				return err
			}
			if _, err := fmt.Fprintf(w, "Heard count  : %d\n", rec.HeardCount); err != nil {
				return err
			}
			if rec.ConfigEntry != "" {
				if _, err := fmt.Fprintf(w, "\nConfig entry:\n%s\n", rec.ConfigEntry); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func prettyDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}
