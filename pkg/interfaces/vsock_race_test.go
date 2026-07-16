// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

//go:build linux && !js

package interfaces

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mdlayher/vsock"

	"quad4/reticulum-go/pkg/common"
)

func TestVSOCKRaceStopSend(t *testing.T) {
	skipIfVSOCKUnavailable(t)

	srv, err := NewVSOCKServerInterface("vsock_race_srv", 0)
	if err != nil {
		t.Fatal(err)
	}
	var n atomic.Int32
	srv.SetPacketCallback(func([]byte, common.NetworkInterface) { n.Add(1) })
	if err := srv.Start(); err != nil {
		t.Skipf("vsock listen failed: %v", err)
	}
	defer srv.Stop()

	cli, err := NewVSOCKClientInterfaceWithRetries("vsock_race_cli", vsock.Local, srv.Port(), true, 20)
	if err != nil {
		t.Fatal(err)
	}
	waitOnline(t, cli, 5*time.Second)
	waitVSOCKSessions(t, srv, 1, 5*time.Second)

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
