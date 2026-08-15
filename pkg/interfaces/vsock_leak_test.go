// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

//go:build linux && !js

package interfaces

import (
	"runtime"
	"testing"
	"time"

	"github.com/mdlayher/vsock"
)

func TestVSOCKNoGoroutineLeak(t *testing.T) {
	skipIfVSOCKUnavailable(t)

	runtime.GC()
	baseline := runtime.NumGoroutine()

	for range 25 {
		srv, err := NewVSOCKServerInterface("vsock_leak_srv", 0)
		if err != nil {
			t.Fatal(err)
		}
		if err := srv.Start(); err != nil {
			t.Skipf("vsock listen failed: %v", err)
		}
		cli, err := NewVSOCKClientInterfaceWithRetries("vsock_leak_cli", vsock.Local, srv.Port(), true, 5)
		if err != nil {
			_ = srv.Stop()
			t.Fatal(err)
		}
		waitOnline(t, cli, 3*time.Second)
		_ = cli.Send([]byte{0x50, 0x01}, "")
		_ = cli.Stop()
		_ = srv.Stop()
	}

	time.Sleep(100 * time.Millisecond)
	runtime.GC()
	final := runtime.NumGoroutine()
	if final > baseline+10 {
		t.Errorf("possible goroutine leak: baseline=%d final=%d", baseline, final)
	}
}
