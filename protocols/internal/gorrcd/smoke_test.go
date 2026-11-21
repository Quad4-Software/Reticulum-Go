// SPDX-License-Identifier: LicenseRef-Reticulum
package gorrcd

import (
	"testing"

	"github.com/Quad4-Software/reticulum-go-protocols/pkg/rrc"
)

func TestSmoke_VersionSet(t *testing.T) {
	if Version == "" {
		t.Fatal("Version must be set")
	}
}

func TestSmoke_DefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.HubName != "rrc" {
		t.Fatalf("hub name=%q", cfg.HubName)
	}
	if cfg.MaxNickBytes == 0 || cfg.MaxMsgBodyBytes == 0 || cfg.MaxRoomNameBytes == 0 {
		t.Fatalf("limits: %+v", cfg)
	}
	if cfg.RoomInviteTimeoutS != 900 {
		t.Fatalf("invite timeout=%v", cfg.RoomInviteTimeoutS)
	}
	lim := cfg.HubLimits()
	if lim.MaxNickBytes != cfg.MaxNickBytes || lim.RateLimitMsgsPerMinute != cfg.RateLimitMsgsPerMinute {
		t.Fatalf("HubLimits mismatch %+v", lim)
	}
}

func TestSmoke_SplitCmd(t *testing.T) {
	got := splitCmd("  /mode  lobby  +k  secret key ")
	if len(got) < 4 || got[0] != "mode" || got[1] != "lobby" {
		t.Fatalf("%v", got)
	}
	if len(splitCmd("")) != 0 || len(splitCmd("/")) != 0 {
		t.Fatal("empty command")
	}
}

func TestSmoke_UDPAddr(t *testing.T) {
	if udpAddr("42950") != "127.0.0.1:42950" {
		t.Fatal(udpAddr("42950"))
	}
	if udpAddr("127.0.0.1:9") != "127.0.0.1:9" {
		t.Fatal(udpAddr("127.0.0.1:9"))
	}
	if udpAddr("") != "" {
		t.Fatal("empty")
	}
}

func TestSmoke_HubPolicySatisfied(t *testing.T) {
	var _ rrc.HubPolicy = (*Service)(nil)
}

func TestSmoke_PathsHonorHome(t *testing.T) {
	t.Setenv("GORRCD_HOME", "/tmp/gorrcd-smoke-home")
	if DefaultHome() != "/tmp/gorrcd-smoke-home" {
		t.Fatal(DefaultHome())
	}
	if DefaultConfigPath() != "/tmp/gorrcd-smoke-home/gorrcd.toml" {
		t.Fatal(DefaultConfigPath())
	}
}
