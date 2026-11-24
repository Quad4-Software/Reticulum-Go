//go:build cgo && (linux || darwin || windows || android || freebsd || openbsd || netbsd || dragonfly)

// SPDX-License-Identifier: LicenseRef-Reticulum
package io_test

import (
	"testing"

	"git.quad4.io/reticulum/Reticulum-Go/protocols/pkg/lxst/audio/io"
)

func TestBackendMiniaudio(t *testing.T) {
	if io.Backend() != "miniaudio" {
		t.Fatalf("backend %q", io.Backend())
	}
}
