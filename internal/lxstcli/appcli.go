// SPDX-License-Identifier: LicenseRef-Reticulum
package appcli

import (
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/identity"
)

const (
	DirMode  = 0o700
	FileMode = 0o600
)

func LoadOrCreateIdentity(path string) (*identity.Identity, error) {
	if path == "" {
		return identity.New()
	}
	if ident, err := identity.FromFile(path); err == nil {
		return ident, nil
	}
	ident, err := identity.New()
	if err != nil {
		return nil, err
	}
	dir := filepath.Dir(path)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, DirMode); err != nil {
			return nil, err
		}
	}
	if err := ident.ToFile(path); err != nil {
		return nil, err
	}
	return ident, nil
}

func WaitSignal() {
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
}
