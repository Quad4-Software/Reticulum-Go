// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

//go:build !js

package interfaces

import (
	"runtime"
	"testing"
	"time"
)

func TestHTTPSNoGoroutineLeak(t *testing.T) {
	runtime.GC()
	baseline := runtime.NumGoroutine()

	for range 20 {
		port := freeHTTPSPort(t)
		srv, err := NewHTTPSServerInterface("https_leak_srv", "127.0.0.1", port, HTTPSServerOptions{
			LongPoll: 200 * time.Millisecond,
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := srv.Start(); err != nil {
			t.Fatal(err)
		}
		port = listenHTTPSPort(t, srv)
		cli, err := NewHTTPSClientInterfaceWithRetries("https_leak_cli", "127.0.0.1", port, true, 5, HTTPSClientOptions{
			LongPoll: 200 * time.Millisecond,
		})
		if err != nil {
			_ = srv.Stop()
			t.Fatal(err)
		}
		waitOnline(t, cli, 5*time.Second)
		_ = cli.Send([]byte{0x50, 0x01}, "")
		_ = cli.Stop()
		_ = srv.Stop()
	}

	time.Sleep(150 * time.Millisecond)
	runtime.GC()
	final := runtime.NumGoroutine()
	if final > baseline+12 {
		t.Errorf("possible goroutine leak: baseline=%d final=%d", baseline, final)
	}
}
