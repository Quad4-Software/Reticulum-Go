// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package rnsutil

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"quad4/reticulum-go/pkg/destination"
	"quad4/reticulum-go/pkg/identity"
	"quad4/reticulum-go/pkg/link"
	"quad4/reticulum-go/pkg/transport"
)

const (
	// RgoshAppName is the native rgosh destination app.
	RgoshAppName = "rgosh"
	// RnshAppName is the Python rnsh destination app (compat mode).
	RnshAppName = "rnsh"
	// DefaultRgoshTimeout is the default path/link wait.
	DefaultRgoshTimeout = 15 * time.Second
)

// RgoshIdentityPath returns the default identity path under storage.
func RgoshIdentityPath(cfgStorage string) string {
	if cfgStorage == "" {
		return ""
	}
	return filepath.Join(cfgStorage, "identities", RgoshAppName)
}

// RnshIdentityPath returns a compat identity path (shared with Python rnsh layout when possible).
func RnshIdentityPath(cfgStorage string) string {
	if cfgStorage == "" {
		return ""
	}
	return filepath.Join(cfgStorage, "identities", RnshAppName)
}

// PrepareRgoshIdentity loads or creates the rgosh identity file.
func PrepareRgoshIdentity(path string) (*identity.Identity, error) {
	return PrepareRNXIdentity(path)
}

// LoadRgoshAllowedIdentities reads allow-list files and CLI hashes.
func LoadRgoshAllowedIdentities(extra []string) ([][]byte, error) {
	var out [][]byte
	seen := map[string]struct{}{}
	add := func(h []byte) {
		k := hex.EncodeToString(h)
		if _, ok := seen[k]; ok {
			return
		}
		seen[k] = struct{}{}
		out = append(out, h)
	}
	home := os.Getenv("HOME")
	candidates := []string{
		"/etc/rgosh/allowed_identities",
		filepath.Join(home, ".config", "rgosh", "allowed_identities"),
		filepath.Join(home, ".rgosh", "allowed_identities"),
		"/etc/rnsh/allowed_identities",
		filepath.Join(home, ".config", "rnsh", "allowed_identities"),
		filepath.Join(home, ".rnsh", "allowed_identities"),
	}
	for _, path := range candidates {
		hashes, err := readAllowedFile(path)
		if err != nil {
			continue
		}
		for _, h := range hashes {
			add(h)
		}
	}
	for _, a := range extra {
		h, err := ParseDestHash(strings.TrimSpace(a))
		if err != nil {
			return nil, fmt.Errorf("allowed identity %q: %w", a, err)
		}
		add(h)
	}
	return out, nil
}

// EstablishRgoshLink waits for a path and opens an outbound link.
// destHash should be the peer's rgosh or rnsh listening hash. Empty appName
// selects rgosh, then DetectShellMode after recall if the hash is rnsh.
func EstablishRgoshLink(ctx context.Context, tr *transport.Transport, destHash []byte, appName string) (*link.Link, error) {
	if err := WaitPath(ctx, tr, destHash); err != nil {
		return nil, fmt.Errorf("path: %w", err)
	}
	remote, err := identity.Recall(destHash)
	if err != nil {
		return nil, fmt.Errorf("recall: %w", err)
	}
	if appName == "" {
		if _, name, ok := DetectShellMode(destHash, remote); ok {
			appName = name
		} else {
			appName = RgoshAppName
		}
	}
	outDest, err := destination.New(remote, destination.Out, destination.Single, appName, tr)
	if err != nil {
		return nil, err
	}
	linkHash := outDest.GetHash()
	if !bytes.Equal(linkHash, destHash) {
		if msg := rgoshAppMismatch(destHash, remote, appName); msg != "" {
			return nil, fmt.Errorf("%s", msg)
		}
		if err := WaitPath(ctx, tr, linkHash); err != nil {
			return nil, fmt.Errorf("path to %s %s: %w", appName, PrettyHex(linkHash), err)
		}
	}
	l := link.NewLink(outDest, tr, nil, nil, nil)
	if err := l.Establish(); err != nil {
		return nil, err
	}
	if err := WaitLinkActive(ctx, l); err != nil {
		l.Teardown()
		return nil, fmt.Errorf("link: %w", err)
	}
	return l, nil
}

// RgoshAppNameForMode returns destination app name for native or compat.
func RgoshAppNameForMode(compat bool) string {
	if compat {
		return RnshAppName
	}
	return RgoshAppName
}

// DetectShellMode reports whether destHash is the rnsh or rgosh destination
// of remote. ok is false when the hash matches neither app.
func DetectShellMode(destHash []byte, remote *identity.Identity) (compat bool, appName string, ok bool) {
	if remote == nil || len(destHash) == 0 {
		return false, "", false
	}
	if bytes.Equal(destHash, destination.Hash(remote, RnshAppName)) {
		return true, RnshAppName, true
	}
	if bytes.Equal(destHash, destination.Hash(remote, RgoshAppName)) {
		return false, RgoshAppName, true
	}
	return false, "", false
}

// rgoshAppMismatch returns a user-facing error when destHash belongs to the
// other shell app (native vs Python rnsh compat).
func rgoshAppMismatch(destHash []byte, remote *identity.Identity, appName string) string {
	if remote == nil || len(destHash) == 0 {
		return ""
	}
	rnshHash := destination.Hash(remote, RnshAppName)
	rgoshHash := destination.Hash(remote, RgoshAppName)
	if appName == RgoshAppName && bytes.Equal(destHash, rnshHash) {
		return fmt.Sprintf("destination %s is rnsh (Python listener)", PrettyHex(destHash))
	}
	if appName == RnshAppName && bytes.Equal(destHash, rgoshHash) {
		return fmt.Sprintf("destination %s is native rgosh", PrettyHex(destHash))
	}
	return ""
}
