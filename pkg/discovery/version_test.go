// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package discovery

import (
	"bytes"
	"testing"
)

// TestVersionTuple matches Python Discovery.version_tuple: each component's
// leading digits contribute; non-digit components stop the parse. A leading
// v/V is stripped so release tags like v1.3.0 compare like Python versions.
func TestVersionTuple(t *testing.T) {
	cases := []struct {
		in   string
		want []int
	}{
		{"1.5.2", []int{1, 5, 2}},
		{"1.5.5", []int{1, 5, 5}},
		{"v1.3.0", []int{1, 3, 0}},
		{"2", []int{2}},
		{"1.10.2", []int{1, 10, 2}},
		{"1.5.2.post1", []int{1, 5, 2}}, // post suffix digits: "post1" has no leading digit -> stop
		{"not.a.version", nil},
		{"", nil},
		{"  ", nil},
		{"1.", []int{1}},
	}
	for _, tc := range cases {
		got := VersionTuple(tc.in)
		if len(got) != len(tc.want) {
			t.Fatalf("VersionTuple(%q)=%v want %v", tc.in, got, tc.want)
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Fatalf("VersionTuple(%q)=%v want %v", tc.in, got, tc.want)
			}
		}
	}
}

func TestVersionAtLeast(t *testing.T) {
	cases := []struct {
		v, m string
		want bool
	}{
		{"1.5.2", "1.5.2", true},
		{"1.5.5", "1.5.2", true},
		{"1.6.0", "1.5.2", true},
		{"1.5.1", "1.5.2", false},
		{"1.4.9", "1.5.2", false},
		{"0.9.9", "1.5.2", false},
		{"v1.5.2", "1.5.2", true},
		{"", "1.5.2", false},
		{"junk", "1.5.2", false},
		{"1.5", "1.5.2", false}, // (1,5) < (1,5,2) in Python tuple order
	}
	for _, tc := range cases {
		if got := VersionAtLeast(tc.v, tc.m); got != tc.want {
			t.Fatalf("VersionAtLeast(%q,%q)=%v want %v", tc.v, tc.m, got, tc.want)
		}
	}
}

// TestAutoconnectMinVersionsOracles checks the table against the Python
// values: only "RNS" >= 1.5.2 in the reference, plus this port's own name.
func TestAutoconnectMinVersionsOracles(t *testing.T) {
	minVer, ok := AutoconnectMinVersions["RNS"]
	if !ok || minVer != "1.5.2" {
		t.Fatalf("RNS min=%q ok=%v, want 1.5.2", minVer, ok)
	}
	if _, ok := AutoconnectMinVersions[ImplementationName]; !ok {
		t.Fatal("own implementation name must be recognized")
	}
	if len(AutoconnectMinVersions) != 2 {
		t.Fatalf("unexpected extra implementations: %v", AutoconnectMinVersions)
	}
}

// TestImplVersionPersistRoundTrip verifies impl_name and version survive the
// persisted discovered-interface record (RNS 1.5.5 keeps them for rnstatus).
func TestImplVersionPersistRoundTrip(t *testing.T) {
	dir := t.TempDir()
	info := &ReceivedAnnounceInfo{
		Info: Info{
			Type:          "BackboneInterface",
			Name:          "ver-peer",
			TransportImpl: "RNS",
			TransportVers: "1.5.5",
			ReachableOn:   "192.0.2.80",
			Port:          4242,
			HasPort:       true,
			Transport:     true,
			TransportID:   bytes.Repeat([]byte{0x71}, 16),
		},
		StampValue:     10,
		RemoteIdentity: bytes.Repeat([]byte{0x72}, 16),
	}
	if err := PersistDiscoveredInterface(dir, info); err != nil {
		t.Fatal(err)
	}
	list, err := ListDiscoveredInterfaces(dir, ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("records=%d want 1", len(list))
	}
	if list[0].ImplName != "RNS" || list[0].Version != "1.5.5" {
		t.Fatalf("impl=%q version=%q", list[0].ImplName, list[0].Version)
	}
	infos, err := LoadPersistedInterfaces(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(infos) != 1 || infos[0].Info.TransportImpl != "RNS" || infos[0].Info.TransportVers != "1.5.5" {
		t.Fatalf("load impl/version missing: %+v", infos)
	}
}

// TestIFACNoneSanitize covers the RNS 1.5.5 fix for invalidly persisted
// literal "None" IFAC values on discovered records.
func TestIFACNoneSanitize(t *testing.T) {
	if got := SanitizeIFACValue("None"); got != "" {
		t.Fatalf("None sanitize got %q", got)
	}
	if got := SanitizeIFACValue("realnet"); got != "realnet" {
		t.Fatalf("value sanitize got %q", got)
	}
	dir := t.TempDir()
	info := &ReceivedAnnounceInfo{
		Info: Info{
			Type:        "BackboneInterface",
			Name:        "ifac-peer",
			ReachableOn: "192.0.2.81",
			Port:        4242,
			HasPort:     true,
			IFACNetname: "None",
			IFACNetkey:  "None",
			TransportID: bytes.Repeat([]byte{0x73}, 16),
		},
		StampValue:     10,
		RemoteIdentity: bytes.Repeat([]byte{0x74}, 16),
	}
	if err := PersistDiscoveredInterface(dir, info); err != nil {
		t.Fatal(err)
	}
	list, err := ListDiscoveredInterfaces(dir, ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].IFACNetname != "" || list[0].IFACNetkey != "" {
		t.Fatalf("IFAC None not sanitized: %+v", list)
	}
}

// TestI2PConfigEntryB32 verifies the RNS 1.5.5 fix that appends .b32.i2p to
// generated I2P peers entries.
func TestI2PConfigEntryB32(t *testing.T) {
	rec := &DiscoveredInterface{
		Type:        "I2PInterface",
		Name:        "i2p-peer",
		ReachableOn: "abcdefghijklmnopqrstuvwxyz234567abcdefghijklmnopqrstuvwx",
		TransportID: bytes.Repeat([]byte{0x75}, 16),
	}
	entry := configEntryForDiscovered(rec)
	if !bytes.Contains([]byte(entry), []byte("peers = abcdefghijklmnopqrstuvwxyz234567abcdefghijklmnopqrstuvwx.b32.i2p")) {
		t.Fatalf("i2p config entry missing b32 suffix: %q", entry)
	}
}
