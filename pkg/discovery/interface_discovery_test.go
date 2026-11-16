// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package discovery

import (
	"testing"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/transport"
)

func TestInterfaceDiscoveryRegistersHandler(t *testing.T) {
	tr := transport.NewTransport(nil)
	d := NewInterfaceDiscovery(tr, DefaultStampValue, nil)
	d.Start()
	if d.handler == nil {
		t.Fatal("handler not registered")
	}
	d.Stop()
	if d.handler != nil {
		t.Fatal("handler not unregistered")
	}
}
