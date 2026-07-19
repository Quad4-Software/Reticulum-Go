// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package librns

import (
	"testing"
	"time"
)

// TestAcceptanceScaffoldMinimum exercises the bindings/SCAFFOLD minimum bar
// against the Go librns facade (version, node lifecycle, identity hash,
// destination hash, event poll TIMEOUT).
func TestAcceptanceScaffoldMinimum(t *testing.T) {
	if ver := Version(); ver == "" {
		t.Fatal("Version empty")
	}

	node, code := NodeCreate("")
	if code != OK || node == 0 {
		t.Fatalf("NodeCreate: code=%d err=%q", code, LastError())
	}
	defer NodeDestroy(node)

	if code := NodeStart(node); code != OK {
		t.Fatalf("NodeStart: %d %s", code, LastError())
	}
	defer NodeStop(node)

	id, code := IdentityGenerate()
	if code != OK || id == 0 {
		t.Fatalf("IdentityGenerate: %d %s", code, LastError())
	}
	defer IdentityDestroy(id)
	hash, code := IdentityHashBytes(id)
	if code != OK {
		t.Fatalf("IdentityHashBytes: %d %s", code, LastError())
	}
	if len(hash) != 16 {
		t.Fatalf("identity hash len=%d want 16", len(hash))
	}

	dest, code := DestinationCreate(node, id, "acceptapp", []string{"service"}, true)
	if code != OK || dest == 0 {
		t.Fatalf("DestinationCreate: %d %s", code, LastError())
	}
	defer DestinationDestroy(dest)
	dh, code := DestinationHash(dest)
	if code != OK {
		t.Fatalf("DestinationHash: %d %s", code, LastError())
	}
	if len(dh) != 16 {
		t.Fatalf("destination hash len=%d want 16", len(dh))
	}

	_, code = EventPoll(node, 10*time.Millisecond)
	if code != ErrTimeout {
		t.Fatalf("EventPoll idle: code=%d want ErrTimeout err=%q", code, LastError())
	}
}
