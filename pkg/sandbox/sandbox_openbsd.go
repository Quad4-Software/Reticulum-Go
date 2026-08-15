// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

//go:build openbsd && !tinygo

package sandbox

import (
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/debug"
)

func applyPlatform(cfg *common.ReticulumConfig) error {
	strict := cfg != nil && cfg.SandboxStrict
	if err := unveilPaths(cfg); err != nil {
		debug.Log(debug.DebugError, "Unveil failed", "error", err)
		if strict {
			return err
		}
	}

	// Pledge promises appropriate for a network daemon:
	//   stdio    - basic I/O
	//   rpath    - read existing files/directories
	//   wpath    - write to existing files/directories
	//   cpath    - create files/directories
	//   inet     - IPv4/IPv6 sockets
	//   dns      - DNS resolution
	//   unix     - Unix domain sockets
	//   fattr    - chmod, chown
	//   proc     - fork/clone (goroutines)
	promises := "stdio rpath wpath cpath inet dns unix fattr proc"
	if err := unix.Pledge(promises, ""); err != nil {
		return err
	}

	debug.Log(debug.DebugInfo, "Sandbox applied", "platform", "openbsd", "promises", promises)
	return nil
}

func unveilPaths(cfg *common.ReticulumConfig) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	paths := []struct {
		path string
		perm string
	}{
		{filepath.Join(home, ".reticulum-go"), "rwc"},
		{"/etc/resolv.conf", "r"},
		{"/etc/hosts", "r"},
		{"/etc/ssl/cert.pem", "r"},
		{"/etc/ssl/certs", "r"},
		{"/tmp", "rwc"},
		{"/var/tmp", "rwc"},
	}

	for _, p := range collectExtraPaths(cfg) {
		perm := "r"
		switch p.kind {
		case pathRWDir:
			perm = "rwc"
		case pathRODir:
			perm = "rx"
		case pathRWFile:
			perm = "rw"
		case pathROFile:
			perm = "rx"
		}
		paths = append(paths, struct {
			path string
			perm string
		}{p.path, perm})
	}

	for _, p := range paths {
		if err := unix.Unveil(p.path, p.perm); err != nil {
			if err != unix.ENOENT {
				debug.Log(debug.DebugError, "Unveil skipped", "path", p.path, "error", err)
			}
		}
	}

	return unix.Unveil("", "")
}
