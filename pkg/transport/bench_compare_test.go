// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

//go:build linux

package transport

import (
	"bufio"
	"math/rand"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/announce"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/common"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/health"
)

// TestBenchMixedFlood is an opt-in benchmark (RET_BENCH=1) that measures
// receiver-side throughput and memory under a mixed announce/data flood,
// for comparison against the Python RNS reference implementation. It
// reports per-round: offered pps, packets processed (RxOK delta),
// announces absorbed (AnnounceOK delta), duplicates, failures, RSS, CPU.
//
//	RET_BENCH=1 RET_BENCH_SECONDS=30 RET_BENCH_ROUNDS=3 \
//	  RET_BENCH_NODES=24 go test ./pkg/transport -run TestBenchMixedFlood -v
func TestBenchMixedFlood(t *testing.T) {
	if os.Getenv("RET_BENCH") != "1" {
		t.Skip("set RET_BENCH=1 to run the comparison benchmark")
	}
	seconds := envInt("RET_BENCH_SECONDS", 30)
	rounds := envInt("RET_BENCH_ROUNDS", 3)
	nodes := envInt("RET_BENCH_NODES", 24)
	injectors := envInt("RET_BENCH_INJECTORS", 8)

	net := buildRandom(t, nodes, 0.15, 0x9e3779b9)
	t.Cleanup(net.close)

	// Converge the mesh with real announces so path tables and dedup
	// state are populated, matching the prof_load harness.
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
		waitForPaths(peers, net.nodes[i].destHash, 30*time.Second)
	}

	cfg := &common.ReticulumConfig{}
	for round := 0; round < rounds; round++ {
		stop := make(chan struct{})
		var wg sync.WaitGroup
		var sent atomic.Int64
		for k := range injectors {
			wg.Go(func() {
				rng := rand.New(rand.NewSource(int64(round)*104729 + int64(k)*7919 + 13))
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
					next := net.nodes[rng.Intn(nodes)].id.Hash()
					dest := net.nodes[rng.Intn(nodes)].destHash
					payload := make([]byte, 96)
					rng.Read(payload)
					pkt := buildHT2(next, dest, 0, payload)
					_ = iface.Send(pkt, "")
					sent.Add(1)

					// ~2% announces with random app_data so every packet
					// is unique: exercises verify + store, not dedup.
					if rng.Intn(50) == 0 {
						an := net.nodes[rng.Intn(nodes)]
						appData := make([]byte, 32)
						rng.Read(appData)
						if a, err := announce.New(an.id, an.destHash, an.destName, appData, false, cfg); err == nil {
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

		time.Sleep(2 * time.Second) // let queues reach steady state
		before := health.Default.TransportCounters()
		rss0, cpu0 := procRSSKB(), cpuSeconds()
		sentBefore := sent.Load()

		time.Sleep(time.Duration(seconds) * time.Second)
		close(stop)
		wg.Wait()

		after := health.Default.TransportCounters()
		d := before.Delta(after)
		elapsed := float64(seconds)
		offered := float64(sent.Load()-sentBefore) / elapsed
		t.Logf("round %d: offered %.0f pkt/s | processed(RxOK) %.0f/s | announces OK %.0f/s dup %.0f/s | unpackFail %d sigFail %d | RSS %.1f MB | CPU %.2f s | goroutines %d",
			round,
			offered,
			float64(d.RxOK)/elapsed,
			float64(d.AnnounceOK)/elapsed,
			float64(d.AnnounceDup)/elapsed,
			d.UnpackFail, d.AnnounceSigFail,
			float64(procRSSKB()-rss0)/1024.0,
			cpuSeconds()-cpu0,
			runtime.NumGoroutine(),
		)
	}
}

func procRSSKB() int64 {
	f, err := os.Open("/proc/self/status")
	if err != nil {
		return 0
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "VmRSS:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				if kb, err := strconv.ParseInt(fields[1], 10, 64); err == nil {
					return kb
				}
			}
		}
	}
	return 0
}

func cpuSeconds() float64 {
	var ru syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &ru); err != nil {
		return 0
	}
	return float64(ru.Utime.Sec) + float64(ru.Utime.Usec)/1e6 +
		float64(ru.Stime.Sec) + float64(ru.Stime.Usec)/1e6
}
