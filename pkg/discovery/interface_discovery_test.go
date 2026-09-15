// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package discovery

import (
	"testing"

	"github.com/Quad4-Software/Reticulum-Go/pkg/transport"
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
