// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package interop

import (
	"bytes"
	"context"
	"strconv"
	"sync"
	"testing"
	"time"

	"quad4/reticulum-go/pkg/destination"
	"quad4/reticulum-go/pkg/identity"
	rlink "quad4/reticulum-go/pkg/link"
	"quad4/reticulum-go/pkg/resource"
	"quad4/reticulum-go/pkg/rnsutil"
)

func TestLiveGoToGoLargeResourceOverUDP(t *testing.T) {
	liveOrSkip(t)
	sizes := []int{2_000, 40_000, 100_000, 300_000, 1_000_000}
	for _, size := range sizes {
		t.Run(strconv.Itoa(size), func(t *testing.T) {
			portA := freeUDPPort(t)
			portB := freeUDPPort(t)
			trA, _, cleanupA := setupGoUDPPeer(t, portA, portB)
			defer cleanupA()
			trB, _, cleanupB := setupGoUDPPeer(t, portB, portA)
			defer cleanupB()

			idListen, err := identity.New()
			if err != nil {
				t.Fatal(err)
			}
			idSend, err := identity.New()
			if err != nil {
				t.Fatal(err)
			}
			dest, err := destination.New(idListen, destination.In, destination.Single, rnsutil.RNCPAppName, trA, rnsutil.RNCPAspect)
			if err != nil {
				t.Fatal(err)
			}
			dest.AcceptsLinks(true)
			received := make(chan []byte, 1)
			var once sync.Once
			dest.SetLinkEstablishedCallback(func(lnk any) {
				l := interopLink(t, lnk)
				_ = l.SetResourceStrategy(rlink.AcceptAll)
				l.SetResourceConcludedCallback(func(p any) {
					once.Do(func() {
						switch v := p.(type) {
						case rlink.IncomingResource:
							received <- v.Data
						case []byte:
							received <- v
						}
					})
				})
			})
			_ = dest.Announce(false, nil, nil)
			time.Sleep(50 * time.Millisecond)
			_ = dest.Announce(false, nil, nil)

			timeout := 60 * time.Second
			if size >= 1_000_000 {
				timeout = 180 * time.Second
			}
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()
			destHash := dest.GetHash()
			if err := rnsutil.WaitPath(ctx, trB, destHash); err != nil {
				t.Fatalf("path: %v", err)
			}
			l, err := rnsutil.EstablishRNCPLink(ctx, trB, destHash)
			if err != nil {
				t.Fatal(err)
			}
			defer l.Teardown()
			if err := l.Identify(idSend); err != nil {
				t.Fatal(err)
			}
			body := bytes.Repeat([]byte("U"), size)
			res, err := resource.New(body, false)
			if err != nil {
				t.Fatal(err)
			}
			_ = res.SetMetadata(map[string]any{"name": []byte("udp.bin")})
			t0 := time.Now()
			if err := l.SendResource(res); err != nil {
				t.Fatal(err)
			}
			wait := max(timeout-5*time.Second, 30*time.Second)
			select {
			case got := <-received:
				if !bytes.Equal(got, body) {
					t.Fatalf("len got=%d want=%d", len(got), len(body))
				}
				t.Logf("ok size=%d dur=%s", size, time.Since(t0))
			case <-time.After(wait):
				t.Fatalf("timeout size=%d after %s", size, time.Since(t0))
			}
		})
	}
}
