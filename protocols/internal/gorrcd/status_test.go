// SPDX-License-Identifier: LicenseRef-Reticulum
package gorrcd

import (
	"testing"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/interfaces"
)

func TestVersionLine(t *testing.T) {
	old := BuildDate
	t.Cleanup(func() { BuildDate = old })
	BuildDate = ""
	if got := VersionLine(); got != Version {
		t.Fatalf("got %q want %q", got, Version)
	}
	BuildDate = "2026-08-14T12:00:00Z"
	want := Version + " (built 2026-08-14T12:00:00Z)"
	if got := VersionLine(); got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestTransportSummarySharedInstanceName(t *testing.T) {
	lc, err := interfaces.NewLocalClientInterface(0, "default", true, nil)
	if err != nil {
		t.Fatal(err)
	}
	label, address := transportSummary(Config{}, []interfaces.Interface{lc})
	if label != "Local shared instance" {
		t.Fatalf("label=%q", label)
	}
	if address != "reticulum shared instance" {
		t.Fatalf("address=%q", address)
	}
}

func TestBuildOperatorSummary(t *testing.T) {
	s := &Service{
		cfg: Config{
			ConfigPath:       "/home/u/.gorrcd/gorrcd.toml",
			IdentityPath:     "/home/u/.gorrcd/hub_identity",
			RoomRegistryPath: "/home/u/.gorrcd/rooms.toml",
			HubName:          "test-hub",
			LogFile:          "/home/u/.gorrcd/gorrcd.log",
			UDPListen:        "127.0.0.1:4242",
			UDPForward:       "127.0.0.1:4243",
		},
		rooms: NewRoomRegistry("", 900),
		stats: NewStats(),
	}
	o := buildOperatorSummary(s)
	if o.HubName != "test-hub" {
		t.Fatalf("hub=%q", o.HubName)
	}
	if o.Transport != "UDPInterface" {
		t.Fatalf("transport=%q", o.Transport)
	}
	if o.Address != "127.0.0.1:4242: 127.0.0.1:4243" {
		t.Fatalf("address=%q", o.Address)
	}
	if o.ConfigPath == "" || o.LogFile == "" {
		t.Fatalf("paths missing: %+v", o)
	}
}

func TestFormatOperatorStatusLine(t *testing.T) {
	line := formatOperatorStatusLine(OperatorSummary{
		PeerCount:     3,
		WelcomedCount: 2,
		RoomCount:     1,
		Memberships:   4,
		MsgsForwarded: 12,
		UptimeSeconds: 90,
	})
	if line == "" {
		t.Fatal("empty line")
	}
	for _, part := range []string{"clients 3", "2 welcomed", "rooms 1", "4 members", "msgs 12", "uptime 1m30s"} {
		if !stringsContains(line, part) {
			t.Fatalf("line=%q missing %q", line, part)
		}
	}
}

func stringsContains(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func TestPrettyTime(t *testing.T) {
	cases := []struct {
		in   float64
		want string
	}{
		{-1, "0s"},
		{0, "0s"},
		{59, "59s"},
		{90, "1m30s"},
		{3665, "1h1m"},
	}
	for _, tc := range cases {
		if got := prettyTime(tc.in); got != tc.want {
			t.Fatalf("prettyTime(%v)=%q want %q", tc.in, got, tc.want)
		}
	}
}

func TestPrettyHubHash(t *testing.T) {
	raw := "aabbccddeeff00112233445566778899"
	dotted := "aabb.ccdd.eeff.0011.2233.4455.6677.8899"
	if got := prettyHubHash(raw); got != raw {
		t.Fatalf("prettyHubHash(%q)=%q", raw, got)
	}
	if got := prettyHubHash(dotted); got != raw {
		t.Fatalf("prettyHubHash(dotted)=%q want %q", got, raw)
	}
}
