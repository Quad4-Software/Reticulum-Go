// SPDX-License-Identifier: LicenseRef-Reticulum
package gorrcd

import (
	"testing"
	"time"

	"github.com/Quad4-Software/reticulum-go-protocols/pkg/rrc"
)

func mustID(seed byte) ID {
	var id ID
	for i := range id {
		id[i] = seed + byte(i)
	}
	return id
}

func mustPeer(seed byte) []byte {
	b := make([]byte, rrc.IdentityLength)
	for i := range b {
		b[i] = seed + byte(i)
	}
	return b
}

func waitCh(t *testing.T, ch <-chan struct{}, label string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(12 * time.Second):
		t.Fatalf("timeout waiting for %s", label)
	}
}

func waitStr(t *testing.T, ch <-chan string, label string) string {
	t.Helper()
	select {
	case s := <-ch:
		return s
	case <-time.After(12 * time.Second):
		t.Fatalf("timeout waiting for %s", label)
		return ""
	}
}

func testConfig() Config {
	cfg := DefaultConfig()
	cfg.RoomRegistryPath = ""
	cfg.ConfigPath = ""
	cfg.IdentityPath = ""
	cfg.AnnounceOnStart = false
	cfg.AnnouncePeriodS = 0
	cfg.PingIntervalS = 0
	cfg.PingTimeoutS = 0
	cfg.EnableResourceTransfer = false
	cfg.LogConsole = false
	cfg.Greeting = ""
	cfg.HubName = "gorrcd-test"
	return cfg
}
