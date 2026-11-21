// SPDX-License-Identifier: LicenseRef-Reticulum
package gorrcd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Quad4-Software/reticulum-go-protocols/pkg/rrc"
)

func TestAcceptance_1_DefaultHubNameIsRrc(t *testing.T) {
	if DefaultConfig().HubName != "rrc" {
		t.Fatal("rrcd default hub name is rrc")
	}
	if Version == "" {
		t.Fatal("version")
	}
}

func TestAcceptance_2_FirstRunPrivateFiles(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "gorrcd.toml")
	id := filepath.Join(dir, "hub_identity")
	rooms := filepath.Join(dir, "rooms.toml")
	rns := filepath.Join(dir, "rns")
	created, err := FirstRun(cfg, id, rooms, rns)
	if err != nil || !created {
		t.Fatalf("created=%v err=%v", created, err)
	}
	created2, err := FirstRun(cfg, id, rooms, rns)
	if err != nil || created2 {
		t.Fatalf("second created=%v err=%v", created2, err)
	}
	raw, err := os.ReadFile(cfg)
	if err != nil || len(raw) == 0 {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(rns, "config")); err != nil {
		t.Fatalf("rns config: %v", err)
	}
}

func TestAcceptance_3_NilPolicyOpenRooms(t *testing.T) {
	var p rrc.HubPolicy
	_ = p
	s := NewService(testConfig(), nil)
	if err := s.AllowJoin(mustPeer(1), "open", nil); err != nil {
		t.Fatal(err)
	}
}

func TestAcceptance_4_UnknownCommandString(t *testing.T) {
	s := NewService(testConfig(), nil)
	s.handleCommand(mustPeer(1), "lobby", "/foo")
	s.handleCommand(mustPeer(1), "", "/FOO")
}

func TestAcceptance_5_LiveWelcomeGate(t *testing.T) {
	if testing.Short() {
		t.Skip("acceptance live gate skipped in -short")
	}
	m := newDaemonMesh(t, 43130, testConfig())
	c := dialDaemon(t, m, 'A', rrc.ClientConfig{Nick: "acc"})
	if c.State() != rrc.ClientActive {
		t.Fatalf("state=%v", c.State())
	}
	if c.Welcome() == nil {
		t.Fatal("WELCOME required")
	}
}

func TestAcceptance_6_TrustedKlineAuth(t *testing.T) {
	s := NewService(testConfig(), nil)
	peer := mustPeer(1)
	s.handleCommand(peer, "", "/kline list")
	s.handleCommand(peer, "", "/reload")
	id, _ := idFrom(peer)
	if s.trust.IsTrusted(id) {
		t.Fatal("untrusted")
	}
}
