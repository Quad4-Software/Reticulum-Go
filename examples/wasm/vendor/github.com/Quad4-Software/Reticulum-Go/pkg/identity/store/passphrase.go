// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package store

import (
	"fmt"
	"io"
	"os"
	"strconv"

	"github.com/Quad4-Software/Reticulum-Go/pkg/securemem"
	"golang.org/x/term"
)

// PassphraseEnv is the environment variable read for identity passphrases.
// PassphraseFDEnv names a file descriptor to read the passphrase from instead
// (for systemd LoadCredential and similar secret-injection mechanisms).
const (
	PassphraseEnv   = "RETICULUM_IDENTITY_PASSPHRASE"
	PassphraseFDEnv = "RETICULUM_IDENTITY_PASSPHRASE_FD"
	// NewPassphraseEnv supplies the new passphrase for non-interactive
	// encrypt and rekey operations, so old and new can differ headlessly.
	NewPassphraseEnv = "RETICULUM_IDENTITY_NEW_PASSPHRASE"

	// AttrIdentityPassphrase marks a backend entry holding the wrap passphrase
	// for an RNE1 identity file (wrapped mode).
	AttrIdentityPassphrase = "identity.passphrase"
)

// PassphraseResolver supplies the passphrase for an encrypted identity file.
// Embedders can install one with SetPassphraseResolver to override the
// default env/fd/backend/prompt chain.
type PassphraseResolver func(path string) ([]byte, error)

var passphraseResolver PassphraseResolver

// SetPassphraseResolver installs a custom resolver consulted before the
// default chain. Pass nil to restore default behavior.
func SetPassphraseResolver(r PassphraseResolver) {
	backendMu.Lock()
	passphraseResolver = r
	backendMu.Unlock()
}

// ResolvePassphrase returns the passphrase for the RNE1 file at path through
// the default chain: custom resolver, env var, passphrase fd, wrap backend,
// then an interactive prompt when stdin is a terminal.
func ResolvePassphrase(path string) ([]byte, error) {
	backendMu.RLock()
	resolver := passphraseResolver
	backendMu.RUnlock()
	if resolver != nil {
		return resolver(path)
	}
	if p := os.Getenv(PassphraseEnv); p != "" {
		return []byte(p), nil
	}
	if fdName := os.Getenv(PassphraseFDEnv); fdName != "" {
		// An explicitly configured fd that fails is an error, not a cue to
		// fall through: a headless deploy that set it would otherwise hang
		// or silently downgrade to a prompt the operator never sees.
		return readPassphraseFD(fdName)
	}
	if p, err := resolveWrapPassphrase(path); err == nil {
		return p, nil
	}
	if term.IsTerminal(int(os.Stdin.Fd())) {
		return promptPassphrase(path)
	}
	return nil, ErrPassphraseRequired
}

func readPassphraseFD(fdName string) ([]byte, error) {
	fdNum, err := strconv.ParseUint(fdName, 10, 31)
	if err != nil {
		return nil, fmt.Errorf("identity store: invalid %s value %q", PassphraseFDEnv, fdName)
	}
	fd := uintptr(fdNum)
	f := os.NewFile(fd, "passphrase-fd")
	if f == nil {
		return nil, fmt.Errorf("identity store: bad fd %s", fdName)
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, 4096))
	if err != nil {
		return nil, err
	}
	return trimPassphrase(data), nil
}

func trimPassphrase(data []byte) []byte {
	for len(data) > 0 && (data[len(data)-1] == '\n' || data[len(data)-1] == '\r') {
		data = data[:len(data)-1]
	}
	return data
}

func promptPassphrase(path string) ([]byte, error) {
	fmt.Fprintf(os.Stderr, "Passphrase for identity %s: ", path)
	p, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return nil, err
	}
	if len(p) == 0 {
		return nil, ErrPassphraseRequired
	}
	return p, nil
}

// PromptNewPassphrase asks for a new passphrase twice on the terminal.
func PromptNewPassphrase(path string) ([]byte, error) {
	fmt.Fprintf(os.Stderr, "New passphrase for identity %s: ", path)
	p1, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return nil, err
	}
	defer securemem.WipeBytes(p1)
	if len(p1) < 8 {
		return nil, fmt.Errorf("passphrase too short (minimum 8 characters)")
	}
	fmt.Fprint(os.Stderr, "Confirm passphrase: ")
	p2, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return nil, err
	}
	defer securemem.WipeBytes(p2)
	if string(p1) != string(p2) {
		return nil, fmt.Errorf("passphrases do not match")
	}
	out := append([]byte(nil), p1...)
	return out, nil
}
