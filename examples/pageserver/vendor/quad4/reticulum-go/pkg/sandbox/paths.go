// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package sandbox

import (
	"path/filepath"
	"strconv"
	"strings"

	"quad4/reticulum-go/pkg/common"
)

type extraPathKind int

const (
	pathRWFile extraPathKind = iota
	pathROFile
	pathRWDir
	pathRODir
)

type extraPath struct {
	path string
	kind extraPathKind
}

func isRouterProfile(cfg *common.ReticulumConfig) bool {
	if cfg == nil {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(cfg.SandboxProfile), common.SandboxProfileRouter)
}

func collectExtraPaths(cfg *common.ReticulumConfig) []extraPath {
	if cfg == nil {
		return nil
	}
	seen := make(map[string]struct{})
	var out []extraPath
	add := func(p string, kind extraPathKind) {
		p = strings.TrimSpace(p)
		if p == "" || p == "." {
			return
		}
		if !filepath.IsAbs(p) {
			return
		}
		key := p + "\x00" + strconv.Itoa(int(kind))
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		out = append(out, extraPath{path: p, kind: kind})
	}
	addFileOrParent := func(p string, fileKind extraPathKind, parentKind extraPathKind) {
		p = strings.TrimSpace(p)
		if p == "" {
			return
		}
		add(p, fileKind)
		parent := filepath.Dir(p)
		if parent != "" && parent != "." && parent != "/" {
			add(parent, parentKind)
		}
	}

	if cfg.ConfigPath != "" {
		add(filepath.Dir(cfg.ConfigPath), pathRWDir)
	}
	if cfg.LogFile != "" {
		add(filepath.Dir(cfg.LogFile), pathRWDir)
	}
	if cfg.NetworkIdentityPath != "" {
		addFileOrParent(cfg.NetworkIdentityPath, pathROFile, pathRODir)
	}
	if cfg.ControlAPISocket != "" {
		addFileOrParent(cfg.ControlAPISocket, pathRWFile, pathRWDir)
	}

	for _, iface := range cfg.Interfaces {
		if iface == nil {
			continue
		}
		dev := strings.TrimSpace(iface.Device)
		if dev == "" {
			dev = strings.TrimSpace(iface.Address)
		}
		if looksLikeDevicePath(dev) {
			add(dev, pathRWFile)
		}
		addCommandPath(iface.Command, add)
		addCommandPath(iface.DiscoveryLocationCmd, add)
		addCommandPath(iface.ReachableOn, add)
		addFileOrParent(iface.CertFile, pathROFile, pathRODir)
		addFileOrParent(iface.KeyFile, pathROFile, pathRODir)
		addFileOrParent(iface.PeerKey, pathROFile, pathRODir)
	}

	for _, p := range cfg.SandboxExtraPaths {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if before, ok := strings.CutSuffix(p, "/"); ok {
			add(before, pathRWDir)
			continue
		}
		add(p, pathRWFile)
		parent := filepath.Dir(p)
		if parent != "" && parent != "." && parent != "/" {
			add(parent, pathRWDir)
		}
	}
	return out
}

func looksLikeDevicePath(p string) bool {
	if !filepath.IsAbs(p) {
		return false
	}
	return strings.HasPrefix(p, "/dev/") || strings.Contains(p, "/tty")
}

func addCommandPath(command string, add func(string, extraPathKind)) {
	command = strings.TrimSpace(command)
	if command == "" {
		return
	}
	fields := strings.Fields(command)
	if len(fields) == 0 {
		return
	}
	bin := fields[0]
	if !filepath.IsAbs(bin) {
		return
	}
	add(bin, pathROFile)
	parent := filepath.Dir(bin)
	if parent != "" && parent != "." && parent != "/" {
		add(parent, pathRODir)
	}
}
