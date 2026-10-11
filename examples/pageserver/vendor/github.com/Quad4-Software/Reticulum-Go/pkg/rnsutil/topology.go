// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package rnsutil

import (
	"fmt"
	"io"
	"sort"
	"time"

	"github.com/Quad4-Software/Reticulum-Go/pkg/transport"
)

// WriteTopologyHuman renders the path table grouped by receiving interface:
// each interface gets a header with path count and hop range, then one row
// per destination showing hops, the next-hop transport hash, and age. Purely
// presentational; pairs with the per-interface announce counters.
func WriteTopologyHuman(w io.Writer, stats transport.InterfaceStatsResponse, paths []transport.PathTableEntry, now time.Time) error {
	if now.IsZero() {
		now = time.Now()
	}

	ifaceStatus := make(map[string]bool, len(stats.Interfaces))
	for i := range stats.Interfaces {
		ifaceStatus[stats.Interfaces[i].Name] = stats.Interfaces[i].Status
	}

	groups := make(map[string][]transport.PathTableEntry)
	for _, p := range paths {
		name := p.Interface
		if name == "" {
			name = "(unknown)"
		}
		groups[name] = append(groups[name], p)
	}

	names := make([]string, 0, len(groups))
	for name := range groups {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool {
		if len(groups[names[i]]) != len(groups[names[j]]) {
			return len(groups[names[i]]) > len(groups[names[j]])
		}
		return names[i] < names[j]
	})

	if _, err := fmt.Fprintf(w, "Topology: %d path%s via %d interface%s\n\n",
		len(paths), plural(len(paths)), len(groups), plural(len(groups))); err != nil {
		return err
	}

	for _, name := range names {
		rows := groups[name]
		minHops, maxHops := 255, 0
		for _, p := range rows {
			if int(p.Hops) < minHops {
				minHops = int(p.Hops)
			}
			if int(p.Hops) > maxHops {
				maxHops = int(p.Hops)
			}
		}
		status := "offline"
		if ifaceStatus[name] {
			status = "online"
		}
		if _, err := fmt.Fprintf(w, "%s (%s) - %d path%s, hops %d-%d\n",
			name, status, len(rows), plural(len(rows)), minHops, maxHops); err != nil {
			return err
		}
		sort.Slice(rows, func(i, j int) bool {
			if rows[i].Hops != rows[j].Hops {
				return rows[i].Hops < rows[j].Hops
			}
			return rows[i].Timestamp > rows[j].Timestamp
		})
		for _, p := range rows {
			heard := now.Sub(time.Unix(int64(p.Timestamp), 0))
			if heard < 0 {
				heard = 0
			}
			line := fmt.Sprintf("  %s  hops=%d", PrettyHex(p.Hash), p.Hops)
			if len(p.Via) > 0 {
				line += fmt.Sprintf("  via %s", PrettyHex(p.Via))
			}
			line += fmt.Sprintf("  heard %s ago", prettyDuration(heard))
			if p.Expires > 0 {
				left := time.Until(time.Unix(int64(p.Expires), 0))
				if left <= 0 {
					line += "  (expired)"
				} else {
					line += fmt.Sprintf("  exp in %s", prettyDuration(left))
				}
			}
			if _, err := fmt.Fprintln(w, line); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintln(w); err != nil {
			return err
		}
	}
	return nil
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
