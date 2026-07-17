// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package rnsutil

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"quad4/reticulum-go/pkg/identity"
	"quad4/reticulum-go/pkg/transport"
)

// Reach stages describe why a destination is or is not reachable.
const (
	ReachStageNoInterfaces = "no_interfaces"
	ReachStageBlackholed   = "blackholed"
	ReachStageNoPath       = "no_path"
	ReachStageUnresponsive = "unresponsive"
	ReachStageReachable    = "reachable"
)

// ReachReport explains local path reachability for a destination hash.
type ReachReport struct {
	DestHash     string   `json:"dest_hash"`
	Stage        string   `json:"stage"`
	Summary      string   `json:"summary"`
	HasPath      bool     `json:"has_path"`
	Unresponsive bool     `json:"unresponsive"`
	Blackholed   bool     `json:"blackholed"`
	Hops         int      `json:"hops,omitempty"`
	Via          string   `json:"via,omitempty"`
	Interface    string   `json:"interface,omitempty"`
	OnlineIfaces int      `json:"online_interfaces"`
	IfaceNames   []string `json:"interface_names,omitempty"`
	Hints        []string `json:"hints"`
}

// DiagnoseReachability inspects transport state for destHash and returns
// a structured explanation of why a path request may fail.
func DiagnoseReachability(tr *transport.Transport, destHash []byte) ReachReport {
	report := ReachReport{
		DestHash: hex.EncodeToString(destHash),
		Hints:    make([]string, 0, 4),
	}
	if tr == nil {
		report.Stage = ReachStageNoInterfaces
		report.Summary = "no transport available"
		report.Hints = append(report.Hints, "start a local node or attach to a shared instance")
		return report
	}

	ifaces := tr.GetInterfaces()
	names := make([]string, 0, len(ifaces))
	online := 0
	for name, iface := range ifaces {
		if iface == nil {
			continue
		}
		names = append(names, name)
		if iface.IsEnabled() && iface.IsOnline() {
			online++
		}
	}
	sort.Strings(names)
	report.IfaceNames = names
	report.OnlineIfaces = online

	if bh := tr.BlackholeTable(); bh != nil && len(destHash) == identity.TruncatedHashLength/8 {
		report.Blackholed = bh.Has(destHash)
	}

	report.HasPath = tr.HasPath(destHash)
	report.Unresponsive = tr.PathIsUnresponsive(destHash)
	if report.HasPath {
		report.Hops = int(tr.HopsTo(destHash))
		if via := tr.NextHop(destHash); len(via) > 0 {
			report.Via = hex.EncodeToString(via)
		}
		if ifn := tr.NextHopInterface(destHash); ifn != "" && ifn != "None" {
			report.Interface = ifn
		}
	}

	switch {
	case online == 0:
		report.Stage = ReachStageNoInterfaces
		report.Summary = "no online network interfaces"
		report.Hints = append(report.Hints,
			"enable at least one interface and confirm it is Up in status",
			"check listen address, peer host, and shared instance connectivity",
		)
	case report.Blackholed:
		report.Stage = ReachStageBlackholed
		report.Summary = "destination identity is blackholed locally"
		report.Hints = append(report.Hints,
			"remove the blackhole with path -unblackhole <hash> if that is intentional",
		)
	case !report.HasPath && report.Unresponsive:
		report.Stage = ReachStageUnresponsive
		report.Summary = "path previously marked unresponsive and no live path is cached"
		report.Hints = append(report.Hints,
			"wait for a fresh announce from the peer or retry after the path TTL",
			"confirm the peer is online and announcing on a shared hub",
		)
	case !report.HasPath:
		report.Stage = ReachStageNoPath
		report.Summary = "no path to destination"
		report.Hints = append(report.Hints,
			"confirm the destination hash is correct and the peer has announced recently",
			"join a common hub or transport-enabled peer so path responses can arrive",
			"use status to verify path request TX/RX rates are non-zero",
		)
		if report.Unresponsive {
			report.Hints = append(report.Hints, "path state is also marked unresponsive")
		}
	case report.Unresponsive:
		report.Stage = ReachStageUnresponsive
		report.Summary = "path is cached but marked unresponsive"
		report.Hints = append(report.Hints,
			"the next hop may be stale. wait for a new announce or drop the path and retry",
		)
	default:
		report.Stage = ReachStageReachable
		report.Summary = "path is available"
		if report.Hops > 0 {
			report.Summary = fmt.Sprintf("path is available (%d hops)", report.Hops)
		}
	}
	return report
}

// WriteReachReportHuman writes a concise reachability diagnosis.
func WriteReachReportHuman(w io.Writer, report ReachReport) error {
	if _, err := fmt.Fprintf(w, "Reachability for %s\n", PrettyHex(mustDecodeHex(report.DestHash))); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  Stage       : %s\n", report.Stage); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  Summary     : %s\n", report.Summary); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  Online ifaces: %d\n", report.OnlineIfaces); err != nil {
		return err
	}
	if len(report.IfaceNames) > 0 {
		if _, err := fmt.Fprintf(w, "  Interfaces  : %s\n", strings.Join(report.IfaceNames, ", ")); err != nil {
			return err
		}
	}
	if report.HasPath {
		via := report.Via
		if via == "" {
			via = "-"
		}
		iface := report.Interface
		if iface == "" {
			iface = "-"
		}
		if _, err := fmt.Fprintf(w, "  Path        : %d hops via %s on %s\n",
			report.Hops, PrettyHex(mustDecodeHex(via)), iface); err != nil {
			return err
		}
	}
	if report.Blackholed {
		if _, err := fmt.Fprintln(w, "  Blackhole   : yes"); err != nil {
			return err
		}
	}
	if report.Unresponsive {
		if _, err := fmt.Fprintln(w, "  Unresponsive: yes"); err != nil {
			return err
		}
	}
	for _, hint := range report.Hints {
		if _, err := fmt.Fprintf(w, "  Hint        : %s\n", hint); err != nil {
			return err
		}
	}
	return nil
}

// WriteReachReportJSON writes the reachability report as JSON.
func WriteReachReportJSON(w io.Writer, report ReachReport) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(report)
}

func mustDecodeHex(s string) []byte {
	if s == "" || s == "-" {
		return nil
	}
	b, err := hex.DecodeString(s)
	if err != nil {
		return nil
	}
	return b
}
