// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package rnsutil

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

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
	NameFilter string
	SortBy     string
	SortAsc    bool
	ShowAll    bool
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
func WriteStatusHuman(w io.Writer, stats transport.InterfaceStatsResponse, linkCount *int, opts StatusOptions) error {
	filter := strings.ToLower(opts.NameFilter)
	for i := range stats.Interfaces {
		st := &stats.Interfaces[i]
		if hideInterface(st.Name, opts.ShowAll) {
			continue
		}
		if filter != "" && !strings.Contains(strings.ToLower(st.Name), filter) {
			continue
		}
		state := "Down"
		if st.Status {
			state = "Up"
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
		if _, err := fmt.Fprintf(w, "  Announces : ↓%.2f/s ↑%.2f/s\n  Path reqs : ↓%.2f/s ↑%.2f/s\n  Held      : %d\n",
			st.IncomingAnnounceFrequency, st.OutgoingAnnounceFrequency,
			st.IncomingPRFrequency, st.OutgoingPRFrequency,
			st.HeldAnnounces,
		); err != nil {
			return err
		}
		if st.BurstActive || st.PRBurstActive {
			if _, err := fmt.Fprintf(w, "  Burst     : announce=%v pathreq=%v\n", st.BurstActive, st.PRBurstActive); err != nil {
				return err
			}
		}
		if st.Clients != nil {
			if _, err := fmt.Fprintf(w, "  Clients   : %d\n", *st.Clients); err != nil {
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
		if _, err := fmt.Fprintf(w, "Links        : %d\n", *linkCount); err != nil {
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
		Name                      string  `json:"name"`
		ShortName                 string  `json:"short_name"`
		Hash                      string  `json:"hash,omitempty"`
		Type                      string  `json:"type"`
		RXB                       uint64  `json:"rxb"`
		TXB                       uint64  `json:"txb"`
		RXS                       float64 `json:"rxs"`
		TXS                       float64 `json:"txs"`
		Status                    bool    `json:"status"`
		Mode                      byte    `json:"mode"`
		Clients                   *int    `json:"clients"`
		Bitrate                   int64   `json:"bitrate"`
		IncomingAnnounceFrequency float64 `json:"incoming_announce_frequency"`
		OutgoingAnnounceFrequency float64 `json:"outgoing_announce_frequency"`
		IncomingPRFrequency       float64 `json:"incoming_pr_frequency"`
		OutgoingPRFrequency       float64 `json:"outgoing_pr_frequency"`
		HeldAnnounces             int     `json:"held_announces"`
		BurstActive               bool    `json:"burst_active"`
		PRBurstActive             bool    `json:"pr_burst_active"`
		I2PB32                    *string `json:"i2p_b32,omitempty"`
		Tunnel                    *string `json:"tunnelstate,omitempty"`
	}
	out := struct {
		Interfaces      []ifaceJSON `json:"interfaces"`
		RXB             uint64      `json:"rxb"`
		TXB             uint64      `json:"txb"`
		RXS             float64     `json:"rxs"`
		TXS             float64     `json:"txs"`
		TransportID     string      `json:"transport_id"`
		TransportUptime float64     `json:"transport_uptime"`
	}{
		Interfaces:      make([]ifaceJSON, 0, len(stats.Interfaces)),
		RXB:             stats.RXB,
		TXB:             stats.TXB,
		RXS:             stats.RXS,
		TXS:             stats.TXS,
		TransportID:     hex.EncodeToString(stats.TransportID),
		TransportUptime: stats.TransportUptime,
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
			Bitrate:                   st.Bitrate,
			IncomingAnnounceFrequency: st.IncomingAnnounceFrequency,
			OutgoingAnnounceFrequency: st.OutgoingAnnounceFrequency,
			IncomingPRFrequency:       st.IncomingPRFrequency,
			OutgoingPRFrequency:       st.OutgoingPRFrequency,
			HeldAnnounces:             st.HeldAnnounces,
			BurstActive:               st.BurstActive,
			PRBurstActive:             st.PRBurstActive,
			I2PB32:                    st.I2PB32,
			Tunnel:                    st.TunnelState,
		})
	}
	enc := json.NewEncoder(w)
	return enc.Encode(out)
}
