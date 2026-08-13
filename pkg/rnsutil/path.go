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
	"time"

	"quad4/reticulum-go/pkg/transport"
)

// BlackholeEntry is a normalized blackhole list row for CLI output.
type BlackholeEntry struct {
	Identity []byte
	Until    float64
	Reason   string
	Source   []byte
}

// WritePathTableHuman writes path table rows in rnpath style.
func WritePathTableHuman(w io.Writer, table []transport.PathTableEntry, filter []byte) (int, error) {
	displayed := 0
	sorted := append([]transport.PathTableEntry(nil), table...)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].Interface != sorted[j].Interface {
			return sorted[i].Interface < sorted[j].Interface
		}
		return sorted[i].Hops < sorted[j].Hops
	})
	for _, path := range sorted {
		if len(filter) > 0 && !bytesEqual(filter, path.Hash) {
			continue
		}
		displayed++
		hopWord := "hops"
		if path.Hops == 1 {
			hopWord = "hop "
		}
		if _, err := fmt.Fprintf(w, "%s is %d %s away via %s on %s expires %s\n",
			PrettyHex(path.Hash), path.Hops, hopWord, PrettyHex(path.Via), path.Interface,
			formatUnixTimestamp(path.Expires)); err != nil {
			return displayed, err
		}
	}
	return displayed, nil
}

// WritePathTableJSON writes the path table as JSON with hashes hex-encoded.
func WritePathTableJSON(w io.Writer, table []transport.PathTableEntry) error {
	type row struct {
		Hash      string  `json:"hash"`
		Timestamp float64 `json:"timestamp"`
		Via       string  `json:"via"`
		Hops      uint8   `json:"hops"`
		Expires   float64 `json:"expires"`
		Interface string  `json:"interface"`
	}
	out := make([]row, 0, len(table))
	for _, p := range table {
		out = append(out, row{
			Hash:      hex.EncodeToString(p.Hash),
			Timestamp: p.Timestamp,
			Via:       hex.EncodeToString(p.Via),
			Hops:      p.Hops,
			Expires:   p.Expires,
			Interface: p.Interface,
		})
	}
	return json.NewEncoder(w).Encode(out)
}

// WriteRateTableHuman writes announce-rate rows in rnpath style.
func WriteRateTableHuman(w io.Writer, table []transport.RateTableEntry, filter []byte) (int, error) {
	displayed := 0
	now := float64(time.Now().Unix())
	for _, e := range table {
		if len(filter) > 0 && !bytesEqual(filter, e.Hash) {
			continue
		}
		displayed++
		lastAgo := formatSeconds(maxFloat(0, now-e.Last))
		hourRate := 0
		if len(e.Timestamps) > 1 {
			span := e.Timestamps[len(e.Timestamps)-1] - e.Timestamps[0]
			if span > 0 {
				hourRate = int(float64(len(e.Timestamps)) * 3600 / span)
			}
		}
		rv := ""
		if e.RateViolations > 0 {
			word := "violation"
			if e.RateViolations != 1 {
				word = "violations"
			}
			rv = fmt.Sprintf(", %d active rate %s", e.RateViolations, word)
		}
		bl := ""
		if e.BlockedUntil > now {
			bl = ", new announces allowed in " + formatSeconds(e.BlockedUntil-now)
		}
		if _, err := fmt.Fprintf(w, "%s last heard %s ago, %d announces/hour%s%s\n",
			PrettyHex(e.Hash), lastAgo, hourRate, rv, bl); err != nil {
			return displayed, err
		}
	}
	return displayed, nil
}

// WriteRateTableJSON writes the rate table as JSON with hashes hex-encoded.
func WriteRateTableJSON(w io.Writer, table []transport.RateTableEntry) error {
	type row struct {
		Hash           string    `json:"hash"`
		Last           float64   `json:"last"`
		RateViolations int       `json:"rate_violations"`
		BlockedUntil   float64   `json:"blocked_until"`
		Timestamps     []float64 `json:"timestamps"`
	}
	out := make([]row, 0, len(table))
	for _, e := range table {
		out = append(out, row{
			Hash:           hex.EncodeToString(e.Hash),
			Last:           e.Last,
			RateViolations: e.RateViolations,
			BlockedUntil:   e.BlockedUntil,
			Timestamps:     e.Timestamps,
		})
	}
	return json.NewEncoder(w).Encode(out)
}

// WriteBlackholeHuman writes blackhole entries in rnpath style.
func WriteBlackholeHuman(w io.Writer, entries []BlackholeEntry, filter string) error {
	now := float64(time.Now().Unix())
	for _, e := range entries {
		untilStr := "indefinitely"
		if e.Until > 0 {
			untilStr = fmt.Sprintf("for %s", formatSeconds(maxFloat(0, e.Until-now)))
		}
		reasonStr := ""
		if e.Reason != "" {
			reasonStr = " (" + truncateRunes(e.Reason, 64) + ")"
		}
		byStr := ""
		if len(e.Source) > 0 {
			byStr = " by " + PrettyHex(e.Source)
		}
		line := fmt.Sprintf("%s blackholed %s%s%s", PrettyHex(e.Identity), untilStr, reasonStr, byStr)
		if filter != "" && !containsFold(line, filter) {
			continue
		}
		if _, err := fmt.Fprintln(w, line); err != nil {
			return err
		}
	}
	return nil
}

// WriteBlackholeJSON writes blackhole entries as JSON.
func WriteBlackholeJSON(w io.Writer, entries []BlackholeEntry) error {
	type row struct {
		Identity string  `json:"identity"`
		Until    float64 `json:"until"`
		Reason   string  `json:"reason"`
		Source   string  `json:"source,omitempty"`
	}
	out := make([]row, 0, len(entries))
	for _, e := range entries {
		out = append(out, row{
			Identity: hex.EncodeToString(e.Identity),
			Until:    e.Until,
			Reason:   e.Reason,
			Source:   hex.EncodeToString(e.Source),
		})
	}
	return json.NewEncoder(w).Encode(out)
}

// NormalizeBlackholeRPC converts RPC map rows into BlackholeEntry values.
func NormalizeBlackholeRPC(raw []map[string]any) []BlackholeEntry {
	out := make([]BlackholeEntry, 0, len(raw))
	for _, m := range raw {
		e := BlackholeEntry{
			Until:  asFloat64(m["until"]),
			Reason: asStringVal(m["reason"]),
		}
		e.Identity = asByteSlice(m["identity"])
		e.Source = asByteSlice(m["source"])
		if len(e.Identity) == 0 {
			continue
		}
		out = append(out, e)
	}
	return out
}

func formatUnixTimestamp(ts float64) string {
	if ts <= 0 {
		return "unknown"
	}
	return time.Unix(int64(ts), 0).UTC().Format("2006-01-02 15:04:05")
}

func truncateRunes(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max-1]) + "…"
}

func containsFold(hay, needle string) bool {
	return needle == "" || strings.Contains(strings.ToLower(hay), strings.ToLower(needle))
}

func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func asFloat64(v any) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case float32:
		return float64(x)
	case int:
		return float64(x)
	case int64:
		return float64(x)
	case uint64:
		return float64(x)
	default:
		return 0
	}
}

func asStringVal(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func asByteSlice(v any) []byte {
	switch x := v.(type) {
	case []byte:
		return append([]byte(nil), x...)
	case string:
		b, err := hex.DecodeString(x)
		if err == nil {
			return b
		}
	}
	return nil
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
