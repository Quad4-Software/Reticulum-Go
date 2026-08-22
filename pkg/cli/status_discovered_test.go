// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package cli

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"quad4/reticulum-go/pkg/discovery"
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
	code := RunStatus([]string{"-config", dir, "-d"}, Options{Stdout: &out, Stderr: &out})
	if code != 0 {
		t.Fatalf("exit=%d out=%s", code, out.String())
	}
	if !strings.Contains(out.String(), "cli-peer") {
		t.Fatalf("output=%q", out.String())
	}
}

func writeTestConfig(path string) error {
	return os.WriteFile(path, []byte("[reticulum]\nshare_instance = no\n"), 0o600)
}
