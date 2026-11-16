// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package transport

import (
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/announce"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/common"
)

// TestProfSimNetworkLoad drives a simulated mesh network under sustained
// packet load while writing pprof CPU, heap, goroutine, mutex and block
// profiles. It only runs when RET_PROF_LOAD=1 is set so normal test runs
// are unaffected.
//
//	RET_PROF_LOAD=1 RET_PROF_SECONDS=45 RET_PROF_NODES=24 \
//	  RET_PROF_OUT=/tmp/rns-prof go test ./pkg/transport -run TestProfSimNetworkLoad -v
func TestProfSimNetworkLoad(t *testing.T) {
	if os.Getenv("RET_PROF_LOAD") != "1" {
		t.Skip("set RET_PROF_LOAD=1 to run the profiling load test")
	}

	seconds := envInt("RET_PROF_SECONDS", 45)
	nodes := envInt("RET_PROF_NODES", 24)
	injectors := envInt("RET_PROF_INJECTORS", 8)
	outDir := envStr("RET_PROF_OUT", filepath.Join(os.TempDir(), "rns-prof"))
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", outDir, err)
	}

	// Finer heap sampling for the duration of the run so small
	// per-packet allocations show up in the alloc_space profile.
	oldRate := runtime.MemProfileRate
	runtime.MemProfileRate = 131072
	defer func() { runtime.MemProfileRate = oldRate }()

	// Enable block and mutex contention sampling; without an explicit
	// rate both profiles come out empty. Rates are coarse enough that the
	// sampler overhead stays well under the load it measures.
	runtime.SetBlockProfileRate(1 << 21)
	oldMutexFrac := runtime.SetMutexProfileFraction(5)
	defer runtime.SetMutexProfileFraction(oldMutexFrac)

	net := buildRandom(t, nodes, 0.15, 0x9e3779b9)
	t.Cleanup(net.close)

	// Phase 1: converge the network with real announces so path tables,
	// seen-announce dedup and relay state are all populated.
	originators := nodes / 3
	if originators < 4 {
		originators = nodes
	}
	for i := range originators {
		net.nodes[i].originateAnnounce(t)
	}
	for i := range originators {
		peers := make([]*simNode, 0, nodes-1)
		for j, node := range net.nodes {
			if j != i {
				peers = append(peers, node)
			}
		}
		_, ok := waitForPaths(peers, net.nodes[i].destHash, 30*time.Second)
		if ok != len(peers) {
			t.Logf("warning: announce %d converged on %d/%d peers", i, ok, len(peers))
		}
	}

	var m1, m2 runtime.MemStats
	runtime.ReadMemStats(&m1)
	g1 := runtime.NumGoroutine()

	// Phase 2: sustained mixed load. Injectors pick a random source node
	// and push DATA packets (path-request shaped wire format) plus the
	// occasional freshly signed announce through a random attached iface.
	stop := make(chan struct{})
	var wg sync.WaitGroup
	var sent atomic.Int64
	for k := range injectors {
		wg.Go(func() {
			rng := rand.New(rand.NewSource(int64(k)*7919 + 13))
			cfg := &common.ReticulumConfig{}
			for {
				select {
				case <-stop:
					return
				default:
				}
				src := net.nodes[rng.Intn(nodes)]
				if len(src.ifaces) == 0 {
					continue
				}
				iface := src.ifaces[rng.Intn(len(src.ifaces))]
				// Next-hop transport id: a random peer identity hash keeps
				// packets on the relayed/foreign-id paths rather than the
				// locally-delivered path.
				next := net.nodes[rng.Intn(nodes)].id.Hash()
				dest := net.nodes[rng.Intn(nodes)].destHash
				payload := make([]byte, 96)
				rng.Read(payload)
				pkt := buildHT2(next, dest, 0, payload)
				_ = iface.Send(pkt, "")
				sent.Add(1)

				// ~2% of sends are fresh announces: Ed25519 signing plus
				// receiver-side verification is the dominant per-packet
				// crypto cost in the protocol.
				if rng.Intn(50) == 0 {
					an := net.nodes[rng.Intn(nodes)]
					a, err := announce.New(an.id, an.destHash, an.destName, []byte(an.destName), false, cfg)
					if err == nil {
						if ap, err := a.CreatePacket(); err == nil {
							for _, ifc := range an.ifaces {
								_ = ifc.Send(ap, "")
							}
						}
					}
				}
			}
		})
	}

	cpuPath := filepath.Join(outDir, "cpu.prof")
	cpuF, err := os.Create(cpuPath)
	if err != nil {
		t.Fatalf("create cpu profile: %v", err)
	}
	if err := pprof.StartCPUProfile(cpuF); err != nil {
		t.Fatalf("start cpu profile: %v", err)
	}

	loadStart := time.Now()
	time.Sleep(time.Duration(seconds) * time.Second)
	close(stop)
	wg.Wait()

	pprof.StopCPUProfile()
	cpuF.Close()

	runtime.ReadMemStats(&m2)
	g2 := runtime.NumGoroutine()

	writeProfile := func(name string, fn func(io.Writer) error) {
		p := filepath.Join(outDir, name)
		f, err := os.Create(p)
		if err != nil {
			t.Errorf("create %s: %v", p, err)
			return
		}
		defer f.Close()
		if err := fn(f); err != nil {
			t.Errorf("write %s: %v", p, err)
		}
	}

	writeProfile("heap.prof", pprof.WriteHeapProfile)
	writeProfile("goroutine.prof", func(w io.Writer) error {
		return pprof.Lookup("goroutine").WriteTo(w, 2)
	})
	writeProfile("mutex.prof", func(w io.Writer) error {
		return pprof.Lookup("mutex").WriteTo(w, 2)
	})
	writeProfile("block.prof", func(w io.Writer) error {
		return pprof.Lookup("block").WriteTo(w, 2)
	})

	total := sent.Load()
	allocMB := float64(m2.TotalAlloc-m1.TotalAlloc) / (1 << 20)
	heapInuseMB := float64(int64(m2.HeapInuse)-int64(m1.HeapInuse)) / (1 << 20)
	t.Logf("load ran %s: %d data packets sent, %.1f MB total alloc (%.1f KB/pkt), heap inuse delta %.1f MB, goroutines %d -> %d",
		time.Since(loadStart).Round(time.Millisecond), total, allocMB, allocMB*1024/float64(maxI64(total, 1)), heapInuseMB, g1, g2)
	for _, p := range []string{"cpu.prof", "heap.prof", "goroutine.prof", "mutex.prof", "block.prof"} {
		t.Logf("profile: %s", filepath.Join(outDir, p))
	}
}

func envInt(name string, def int) int {
	if s := os.Getenv(name); s != "" {
		if n, err := strconv.Atoi(s); err == nil {
			return n
		}
	}
	return def
}

func envStr(name, def string) string {
	if s := os.Getenv(name); s != "" {
		return s
	}
	return def
}

func maxI64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
