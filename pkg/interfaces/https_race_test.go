// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

//go:build !js

package interfaces

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"quad4/reticulum-go/pkg/common"
)

func TestHTTPSRaceStopSend(t *testing.T) {
	port := freeHTTPSPort(t)
	srv, err := NewHTTPSServerInterface("https_race_srv", "127.0.0.1", port, HTTPSServerOptions{
		Path:     "/rns",
		LongPoll: 300 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	var n atomic.Int32
	srv.SetPacketCallback(func([]byte, common.NetworkInterface) { n.Add(1) })
	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}
	defer srv.Stop()
	port = listenHTTPSPort(t, srv)

	cli, err := NewHTTPSClientInterfaceWithRetries("https_race_cli", "127.0.0.1", port, true, 20, HTTPSClientOptions{
		Path:     "/rns",
		LongPoll: 300 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	waitOnline(t, cli, 5*time.Second)
	waitHTTPSPeers(t, srv, 1, 5*time.Second)

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := range 60 {
			_ = cli.Send([]byte{0x50, byte(i)}, "")
			time.Sleep(2 * time.Millisecond)
		}
	}()
	go func() {
		defer wg.Done()
		time.Sleep(40 * time.Millisecond)
		_ = cli.Stop()
	}()
	wg.Wait()
	_ = n.Load()
}
