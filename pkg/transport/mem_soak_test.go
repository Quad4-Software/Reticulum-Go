// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package transport

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"strconv"
	"testing"
	"time"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/announce"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/common"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/identity"
)

// readRSSKiB reports the process resident set from /proc/self/status.
func readRSSKiB() int64 {
	data, err := os.ReadFile("/proc/self/status")
	if err != nil {
		return -1
	}
	for _, line := range splitLines(data) {
		if len(line) > 6 && string(line[:6]) == "VmRSS:" {
			var kb int64
			_, _ = fmt.Sscanf(string(line), "VmRSS: %d kB", &kb)
			return kb
		}
	}
	return -1
}

// soakDestHash mirrors the destination hash formula used by the sim
// harness: sha256(name-hash prefix + identity hash) truncated to 16.
func soakDestHash(name string, idHash []byte) []byte {
	nameHash := sha256.Sum256([]byte(name))
	full := sha256.Sum256(append(nameHash[:10], idHash...))
	return full[:16]
}

func splitLines(b []byte) [][]byte {
	var out [][]byte
	start := 0
	for i, c := range b {
		if c == '\n' {
			out = append(out, b[start:i])
			start = i + 1
		}
	}
	if start < len(b) {
		out = append(out, b[start:])
	}
	return out
}

// TestTransportMemorySoak feeds SOAK_DESTS distinct-destination
// announces into one transport and records RSS, heap and table sizes.
// It writes a CSV and a heap profile to SOAK_DIR. Run with:
//
//	SOAK_DESTS=200000 SOAK_DIR=/tmp/mem-161 go test ./pkg/transport -run TestTransportMemorySoak -v
func TestTransportMemorySoak(t *testing.T) {
	v := os.Getenv("SOAK_DESTS")
	if v == "" {
		t.Skip("memory soak is opt-in, set SOAK_DESTS")
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 1 {
		t.Fatalf("SOAK_DESTS=%q invalid", v)
	}
	outDir := os.Getenv("SOAK_DIR")
	if outDir == "" {
		outDir = t.TempDir()
	}
	storage := ""
	if v := os.Getenv("SOAK_STORAGE"); v != "" {
		storage = v
	}

	cfg := unstampedTestConfig(true)
	if v := os.Getenv("SOAK_MAX_PATHS"); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			cfg.MaxInMemoryPaths = p
			cfg.MaxInMemoryPathsSet = true
		}
	}
	if storage != "" {
		cfg.ConfigPath = filepath.Join(storage, "config")
	}
	tr := NewTransport(cfg)
	id, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	tr.SetIdentity(id)
	iface := newSimIface("soak0")
	if err := tr.RegisterInterface("soak0", iface); err != nil {
		t.Fatal(err)
	}
	defer tr.Close()
	defer iface.stop()

	csv, err := os.Create(filepath.Join(outDir, "soak.csv"))
	if err != nil {
		t.Fatal(err)
	}
	defer csv.Close()
	fmt.Fprintln(csv, "dests,rss_kib,heap_alloc_kib,heap_sys_kib,stack_kib,goroutines,paths,announce_table,announce_cache,seen,hashlist")

	sample := func(i int) {
		var ms runtime.MemStats
		runtime.ReadMemStats(&ms)
		stats := tr.MemoryStats()
		fmt.Fprintf(csv, "%d,%d,%d,%d,%d,%d,%d,%d,%d,%d,%d\n",
			i, readRSSKiB(), ms.HeapAlloc/1024, ms.HeapSys/1024, ms.StackInuse/1024,
			runtime.NumGoroutine(), stats.Paths, stats.AnnounceTable,
			stats.AnnouncePacketCache, stats.SeenAnnounces, stats.PacketHashes)
	}
	sample(0)

	start := time.Now()
	step := 10000
	for i := 1; i <= n; i++ {
		src, err := identity.New()
		if err != nil {
			t.Fatal(err)
		}
		name := fmt.Sprintf("soak.dest.%d", i)
		destHash := soakDestHash(name, src.Hash())
		a, err := announce.New(src, destHash, name, []byte(name), false, &common.ReticulumConfig{})
		if err != nil {
			t.Fatal(err)
		}
		pkt, err := a.CreatePacket()
		if err != nil {
			t.Fatal(err)
		}
		tr.handleInboundPacket(pkt, iface, false)
		if i%step == 0 {
			runtime.GC()
			sample(i)
			t.Logf("%d dests rss=%dKiB elapsed=%s", i, readRSSKiB(), time.Since(start).Round(time.Second))
		}
	}

	f, err := os.Create(filepath.Join(outDir, "heap.pprof"))
	if err == nil {
		_ = pprof.WriteHeapProfile(f)
		f.Close()
	}
	af, err := os.Create(filepath.Join(outDir, "allocs.pprof"))
	if err == nil {
		pprof.Lookup("allocs").WriteTo(af, 0)
		af.Close()
	}
}
