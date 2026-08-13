// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package sandbox

import (
	"testing"

	"quad4/reticulum-go/pkg/common"
)

func TestCollectExtraPathsFromConfig(t *testing.T) {
	cfg := &common.ReticulumConfig{
		ConfigPath:       "/var/lib/reticulum-go/config",
		LogFile:          "/var/log/rns/daemon.log",
		ControlAPISocket: "/run/reticulum-go/control.sock",
		SandboxExtraPaths: []string{
			"/opt/rns/extra",
			"/opt/rns/extra",
		},
		Interfaces: map[string]*common.InterfaceConfig{
			"radio": {
				Device:               "/dev/ttyUSB0",
				Command:              "/usr/local/bin/pipehub --foo",
				DiscoveryLocationCmd: "/opt/geo/loc.sh",
				CertFile:             "/etc/rns/cert.pem",
				KeyFile:              "/etc/rns/key.pem",
			},
		},
	}
	got := collectExtraPaths(cfg)
	want := map[string]extraPathKind{
		"/var/lib/reticulum-go":          pathRWDir,
		"/var/log/rns":                   pathRWDir,
		"/run/reticulum-go/control.sock": pathRWFile,
		"/run/reticulum-go":              pathRWDir,
		"/dev/ttyUSB0":                   pathRWFile,
		"/usr/local/bin/pipehub":         pathROFile,
		"/usr/local/bin":                 pathRODir,
		"/opt/geo/loc.sh":                pathROFile,
		"/opt/geo":                       pathRODir,
		"/etc/rns/cert.pem":              pathROFile,
		"/etc/rns":                       pathRODir,
		"/etc/rns/key.pem":               pathROFile,
		"/opt/rns/extra":                 pathRWFile,
		"/opt/rns":                       pathRWDir,
	}
	if len(got) < len(want) {
		t.Fatalf("got %d paths, want at least %d: %+v", len(got), len(want), got)
	}
	have := make(map[string]extraPathKind, len(got))
	for _, p := range got {
		have[p.path] = p.kind
	}
	for path, kind := range want {
		gotKind, ok := have[path]
		if !ok {
			t.Errorf("missing path %s", path)
			continue
		}
		if gotKind != kind {
			t.Errorf("path %s kind %v, want %v", path, gotKind, kind)
		}
	}
}

func TestCollectExtraPathsSkipsRelative(t *testing.T) {
	cfg := &common.ReticulumConfig{
		Interfaces: map[string]*common.InterfaceConfig{
			"pipe": {Command: "cat"},
		},
		SandboxExtraPaths: []string{"relative/path"},
	}
	if got := collectExtraPaths(cfg); len(got) != 0 {
		t.Fatalf("relative paths must be skipped, got %+v", got)
	}
}

func TestIsRouterProfile(t *testing.T) {
	if isRouterProfile(nil) {
		t.Fatal("nil config is not router")
	}
	if isRouterProfile(&common.ReticulumConfig{}) {
		t.Fatal("empty profile is full")
	}
	if isRouterProfile(&common.ReticulumConfig{SandboxProfile: common.SandboxProfileFull}) {
		t.Fatal("full is not router")
	}
	if !isRouterProfile(&common.ReticulumConfig{SandboxProfile: "Router"}) {
		t.Fatal("Router should match")
	}
}
