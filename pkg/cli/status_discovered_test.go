// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package cli

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/Quad4-Software/Reticulum-Go/pkg/discovery"
)

func TestRunStatusDiscoveredList(t *testing.T) {
	dir := t.TempDir()
	cfgPath := dir + "/config"
	if err := writeTestConfig(cfgPath); err != nil {
		t.Fatal(err)
	}
	storage := dir + "/storage"
	info := &discovery.ReceivedAnnounceInfo{
		StampValue: 10,
		Hops:       1,
		Info: discovery.Info{
			Type:        "BackboneInterface",
			Name:        "cli-peer",
			ReachableOn: "192.0.2.50",
			Port:        4242,
			HasPort:     true,
			Transport:   true,
			TransportID: bytes.Repeat([]byte{0x5a}, 16),
		},
		RemoteIdentity: bytes.Repeat([]byte{0x6b}, 16),
	}
	if err := discovery.PersistDiscoveredInterface(storage, info); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	code := RunStatus([]string{"-config", dir, "-d", "-show-unknown"}, Options{Stdout: &out, Stderr: &out})
	if code != 0 {
		t.Fatalf("exit=%d out=%s", code, out.String())
	}
	if !strings.Contains(out.String(), "cli-peer") {
		t.Fatalf("output=%q", out.String())
	}
	if !strings.Contains(out.String(), "Unknown") {
		t.Fatalf("expected Unknown stack output=%q", out.String())
	}
}

// TestRunStatusDiscoveredStackFiltering covers the RNS 1.5.5 Stack field and
// the --show-unknown default hiding of entries without impl info.
func TestRunStatusDiscoveredStackFiltering(t *testing.T) {
	dir := t.TempDir()
	cfgPath := dir + "/config"
	if err := writeTestConfig(cfgPath); err != nil {
		t.Fatal(err)
	}
	storage := dir + "/storage"
	info := &discovery.ReceivedAnnounceInfo{
		StampValue: 10,
		Hops:       1,
		Info: discovery.Info{
			Type:          "BackboneInterface",
			Name:          "stacked-peer",
			TransportImpl: "RNS",
			TransportVers: "1.5.5",
			ReachableOn:   "192.0.2.51",
			Port:          4242,
			HasPort:       true,
			Transport:     true,
			TransportID:   bytes.Repeat([]byte{0x5b}, 16),
		},
		RemoteIdentity: bytes.Repeat([]byte{0x6c}, 16),
	}
	if err := discovery.PersistDiscoveredInterface(storage, info); err != nil {
		t.Fatal(err)
	}
	noImpl := &discovery.ReceivedAnnounceInfo{
		StampValue: 10,
		Hops:       1,
		Info: discovery.Info{
			Type:        "TCPServerInterface",
			Name:        "anonymous-peer",
			ReachableOn: "192.0.2.52",
			Port:        4243,
			HasPort:     true,
			Transport:   true,
			TransportID: bytes.Repeat([]byte{0x5c}, 16),
		},
		RemoteIdentity: bytes.Repeat([]byte{0x6d}, 16),
	}
	if err := discovery.PersistDiscoveredInterface(storage, noImpl); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	code := RunStatus([]string{"-config", dir, "-d"}, Options{Stdout: &out, Stderr: &out})
	if code != 0 {
		t.Fatalf("exit=%d out=%s", code, out.String())
	}
	if !strings.Contains(out.String(), "stacked-peer") {
		t.Fatalf("impl-signed peer should be listed, output=%q", out.String())
	}
	if !strings.Contains(out.String(), "Stack        : RNS 1.5.5") {
		t.Fatalf("expected Stack line, output=%q", out.String())
	}
	if strings.Contains(out.String(), "anonymous-peer") {
		t.Fatalf("entry without impl info must be hidden by default, output=%q", out.String())
	}

	out.Reset()
	code = RunStatus([]string{"-config", dir, "-d", "-show-unknown"}, Options{Stdout: &out, Stderr: &out})
	if code != 0 {
		t.Fatalf("exit=%d out=%s", code, out.String())
	}
	if !strings.Contains(out.String(), "anonymous-peer") {
		t.Fatalf("--show-unknown must reveal entry, output=%q", out.String())
	}
}

func writeTestConfig(path string) error {
	return os.WriteFile(path, []byte("[reticulum]\nshare_instance = no\n"), 0o600)
}
