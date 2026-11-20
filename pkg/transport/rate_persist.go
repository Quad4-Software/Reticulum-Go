// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package transport

import (
	"bytes"
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
	"sort"
	"time"

	"git.quad4.io/reticulum/Reticulum-Go/internal/storage"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/debug"
)

// RNS 1.6.1 RNS.Storage.AnnounceRateTable persist format ("RNSART").
// The Go transport keeps per-interface egress rate histories instead of a
// single global columnar table. On save the histories are merged per
// destination and on load they seed each interface's egress control.

const (
	ratePersistMagic   = "RNSART"
	ratePersistVersion = 0x01
	rateTableMaxTS     = 16 // Python Transport.MAX_RATE_TIMESTAMPS
	rateTableKeyLen    = 16 // RNS.Identity.TRUNCATED_HASHLENGTH//8
)

type rateTableEntry struct {
	last       float64
	blocked    float64
	viol       int32
	timestamps []float64 // chronological, at most rateTableMaxTS
}

func announceRateTablePath(configPath string) (string, error) {
	dir, err := storage.DataDir(configPath)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "announce_rate_table"), nil
}

func marshalRateTable(entries map[[16]byte]rateTableEntry) []byte {
	keys := make([][16]byte, 0, len(entries))
	for k := range entries {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		return bytes.Compare(keys[i][:], keys[j][:]) < 0
	})
	count := len(keys)
	buf := new(bytes.Buffer)
	buf.WriteString(ratePersistMagic)
	buf.WriteByte(ratePersistVersion)
	buf.WriteByte(0x00)
	buf.WriteByte(rateTableMaxTS)
	buf.WriteByte(rateTableKeyLen)
	buf.Write([]byte{0, 0})
	_ = binary.Write(buf, binary.LittleEndian, uint32(count)) // #nosec G115 -- entry count bounded by the rate table size //nolint:errcheck
	for _, k := range keys {
		buf.Write(k[:])
	}
	for _, k := range keys {
		_ = binary.Write(buf, binary.LittleEndian, entries[k].last) //nolint:errcheck
	}
	for _, k := range keys {
		_ = binary.Write(buf, binary.LittleEndian, entries[k].blocked) //nolint:errcheck
	}
	for _, k := range keys {
		_ = binary.Write(buf, binary.LittleEndian, entries[k].viol) //nolint:errcheck
	}
	for _, k := range keys {
		buf.WriteByte(byte(len(entries[k].timestamps))) // #nosec G115 -- timestamps capped at rateTableMaxTS (16)
	}
	for _, k := range keys {
		buf.WriteByte(byte(len(entries[k].timestamps) % rateTableMaxTS)) // #nosec G115 -- result bounded by the modulus
	}
	for _, k := range keys {
		ts := entries[k].timestamps
		for i := 0; i < rateTableMaxTS; i++ {
			v := 0.0
			if i < len(ts) {
				v = ts[i]
			}
			_ = binary.Write(buf, binary.LittleEndian, v) //nolint:errcheck
		}
	}
	return buf.Bytes()
}

func unmarshalRateTable(blob []byte) (map[[16]byte]rateTableEntry, bool) {
	if len(blob) < 16 || string(blob[:6]) != ratePersistMagic ||
		blob[6] != ratePersistVersion || blob[7] != 0x00 {
		return nil, false
	}
	fileMaxTS := int(blob[8])
	keyLen := int(blob[9])
	count := int(binary.LittleEndian.Uint32(blob[12:16]))
	if fileMaxTS < 1 || fileMaxTS > 255 || keyLen != rateTableKeyLen {
		return nil, false
	}
	if len(blob) != 16+count*(keyLen+22+fileMaxTS*8) {
		return nil, false
	}
	off := 16
	keys := make([][16]byte, count)
	for i := range keys {
		copy(keys[i][:], blob[off+i*keyLen:off+(i+1)*keyLen])
	}
	off += count * keyLen
	readF64 := func() []float64 {
		out := make([]float64, count)
		for i := range out {
			out[i] = math.Float64frombits(binary.LittleEndian.Uint64(blob[off+i*8 : off+i*8+8]))
		}
		off += count * 8
		return out
	}
	last := readF64()
	blocked := readF64()
	viol := make([]int32, count)
	for i := range viol {
		viol[i] = int32(binary.LittleEndian.Uint32(blob[off+i*4 : off+i*4+4])) // #nosec G115 -- file field is int32; bit reinterpretation intended
	}
	off += count * 4
	tsLen := make([]int, count)
	for i := range tsLen {
		tsLen[i] = int(blob[off+i])
	}
	off += count
	tsHead := make([]int, count)
	for i := range tsHead {
		tsHead[i] = int(blob[off+i])
	}
	off += count
	for i := range tsLen {
		if tsLen[i] > fileMaxTS || tsHead[i] >= fileMaxTS {
			return nil, false
		}
	}
	tsBase := off
	out := make(map[[16]byte]rateTableEntry, count)
	for i := 0; i < count; i++ {
		base := tsBase + i*fileMaxTS*8
		ring := make([]float64, fileMaxTS)
		for j := 0; j < fileMaxTS; j++ {
			ring[j] = math.Float64frombits(binary.LittleEndian.Uint64(blob[base+j*8 : base+j*8+8]))
		}
		var chronological []float64
		if tsLen[i] < fileMaxTS {
			chronological = append(chronological, ring[:tsLen[i]]...)
		} else {
			chronological = append(chronological, ring[tsHead[i]:]...)
			chronological = append(chronological, ring[:tsHead[i]]...)
		}
		if len(chronological) > rateTableMaxTS {
			chronological = chronological[len(chronological)-rateTableMaxTS:]
		}
		out[keys[i]] = rateTableEntry{
			last:       last[i],
			blocked:    blocked[i],
			viol:       viol[i],
			timestamps: chronological,
		}
	}
	return out, true
}

// loadAnnounceRateTable reads the persisted rate table and stashes it for
// seeding egress controls. Mirrors the Python condition: only when transport
// is enabled and this is not a shared-instance client.
func (t *Transport) loadAnnounceRateTable() {
	if t.config == nil || !t.transportEnabled() || t.config.ConnectedToSharedInstance {
		return
	}
	path, err := announceRateTablePath(configPath(t.config))
	if err != nil {
		return
	}
	blob, err := os.ReadFile(path) // #nosec G304 -- path under the node's own config dir
	if err != nil {
		return
	}
	entries, ok := unmarshalRateTable(blob)
	if !ok {
		debug.Log(debug.DebugWarning, "Could not load announce rate table from storage, initialized empty")
		return
	}
	t.rateTableSeed = entries
	debug.Log(debug.DebugVerbose, "Loaded announce rate table from storage", "entries", len(entries))
}

// saveAnnounceRateTable persists the merged rate table. Called with the same
// gating as Python save_announce_rate_table.
func (t *Transport) saveAnnounceRateTable() {
	if t.config == nil || !t.transportEnabled() || t.config.ConnectedToSharedInstance {
		return
	}
	if t.pathPersistMemory.Load() {
		return
	}
	path, err := announceRateTablePath(configPath(t.config))
	if err != nil {
		return
	}
	merged := make(map[[16]byte]rateTableEntry)
	if t.ifaceStates != nil {
		for _, e := range t.ifaceStates.snapshot() {
			if e.state == nil || e.state.egress == nil {
				continue
			}
			for key, ts := range e.state.egress.Snapshot() {
				ent := merged[key]
				for _, v := range ts {
					f := float64(v.UnixNano()) / 1e9
					if f > ent.last {
						ent.last = f
					}
					ent.timestamps = append(ent.timestamps, f)
				}
				merged[key] = ent
			}
		}
	}
	for key, ent := range merged {
		sort.Float64s(ent.timestamps)
		seen := make(map[float64]bool)
		dedup := ent.timestamps[:0]
		for _, v := range ent.timestamps {
			if !seen[v] {
				seen[v] = true
				dedup = append(dedup, v)
			}
		}
		if len(dedup) > rateTableMaxTS {
			dedup = dedup[len(dedup)-rateTableMaxTS:]
		}
		ent.timestamps = dedup
		merged[key] = ent
	}
	data := marshalRateTable(merged)
	if err := storage.AtomicWriteFile(path, data, 0o600); err != nil {
		debug.Log(debug.DebugError, "Could not persist announce rate table", "path", path, "error", err)
		return
	}
	debug.Log(debug.DebugVerbose, "Saved announce rate table to storage", "entries", len(merged))
}

// seedEgressRateHistory applies a persisted rate table snapshot to a new
// egress control. Used after loadAnnounceRateTable.
func (t *Transport) seedEgressRateHistory(st *ifaceState) {
	if st == nil || st.egress == nil || len(t.rateTableSeed) == 0 {
		return
	}
	hist := make(map[[16]byte][]time.Time, len(t.rateTableSeed))
	for key, ent := range t.rateTableSeed {
		ts := make([]time.Time, 0, len(ent.timestamps))
		for _, v := range ent.timestamps {
			if v > 0 {
				ts = append(ts, time.Unix(0, int64(v*1e9)))
			}
		}
		if len(ts) > 0 {
			hist[key] = ts
		}
	}
	st.egress.Restore(hist)
}
