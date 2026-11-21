// SPDX-License-Identifier: LicenseRef-Reticulum
package lxmf

import (
	"bytes"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/common"
)

func TestChaos_ParallelPaperURI(t *testing.T) {
	if testing.Short() {
		t.Skip("chaos skipped in -short")
	}
	const workers = 24
	const iters = 100
	var wg sync.WaitGroup
	var fails atomic.Int64
	for w := range workers {
		wg.Add(1)
		go func(seed int) {
			defer wg.Done()
			payload := bytes.Repeat([]byte{byte(seed)}, 32+seed%64)
			for range iters {
				uri, err := PaperURI(payload)
				if err != nil {
					fails.Add(1)
					return
				}
				got, err := DecodePaperURI(uri)
				if err != nil || !bytes.Equal(got, payload) {
					fails.Add(1)
					return
				}
			}
		}(w)
	}
	wg.Wait()
	if fails.Load() != 0 {
		t.Fatalf("failures=%d", fails.Load())
	}
}

func TestChaos_MessengerConcurrentSend(t *testing.T) {
	if testing.Short() {
		t.Skip("chaos skipped in -short")
	}

	mesh := newLXMFMesh(t, 42710)
	got := make(chan string, 64)
	mesh.m2.SetMessageHandler(func(msg *LXMessage, _ common.NetworkInterface) {
		select {
		case got <- msg.ContentString():
		default:
		}
	})

	var wg sync.WaitGroup
	var fails atomic.Int64
	const n = 8
	wg.Add(n)
	for i := range n {
		go func(i int) {
			defer wg.Done()
			body := "chaos-" + strconv.Itoa(i)
			if _, err := mesh.m1.SendText(mesh.h2, "c", body); err != nil {
				fails.Add(1)
			}
		}(i)
	}
	wg.Wait()
	if fails.Load() != 0 {
		t.Fatalf("send failures=%d", fails.Load())
	}

	deadline := time.Now().Add(15 * time.Second)
	seen := 0
	for seen < n && time.Now().Before(deadline) {
		select {
		case <-got:
			seen++
		case <-time.After(200 * time.Millisecond):
		}
	}
	if seen < n/2 {
		t.Fatalf("received only %d/%d under chaos", seen, n)
	}
}

func TestChaos_ParallelAnnounceEncode(t *testing.T) {
	if testing.Short() {
		t.Skip("chaos skipped in -short")
	}
	var wg sync.WaitGroup
	var fails atomic.Int64
	for w := range 32 {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := range 50 {
				raw, err := EncodeAnnounceAppDataV5("n"+strconv.Itoa(w), int64(i%16))
				if err != nil {
					fails.Add(1)
					return
				}
				name, err := DisplayNameFromAppData(raw)
				if err != nil || name == "" {
					fails.Add(1)
					return
				}
			}
		}(w)
	}
	wg.Wait()
	if fails.Load() != 0 {
		t.Fatalf("failures=%d", fails.Load())
	}
}
