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

func TestWebTransportRaceStopSend(t *testing.T) {
	port := freeWTPort(t)
	srv, err := NewWebTransportServerInterface("wt_race_srv", "127.0.0.1", port, "/rns", WebTransportServerOptions{})
	if err != nil {
		t.Fatal(err)
	}
	var n atomic.Int32
	srv.SetPacketCallback(func([]byte, common.NetworkInterface) { n.Add(1) })
	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}
	defer srv.Stop()
	port = listenWTPort(t, srv)

	cli, err := NewWebTransportClientInterfaceWithRetries("wt_race_cli", "127.0.0.1", port, "/rns", true, 20, WebTransportClientOptions{})
	if err != nil {
		t.Fatal(err)
	}

	waitOnline(t, cli, 5*time.Second)
	waitWTSessions(t, srv, 1, 5*time.Second)

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := range 100 {
			_ = cli.Send([]byte{byte(i)}, "")
			time.Sleep(time.Millisecond)
		}
	}()
	go func() {
		defer wg.Done()
		time.Sleep(30 * time.Millisecond)
		_ = cli.Stop()
	}()
	wg.Wait()
	_ = n.Load()
}
