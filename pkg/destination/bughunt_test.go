// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package destination

import (
	"testing"

	"quad4/reticulum-go/pkg/identity"
)

// TestBughuntHandleRequestHonorsAllowNone ensures HandleRequest enforces the
// same ACL as GetRequestHandler (pageserver and other callers use HandleRequest).
func TestBughuntHandleRequestHonorsAllowNone(t *testing.T) {
	id, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	d, err := New(id, In, Single, "bughunt", &mockTransport{}, "acl")
	if err != nil {
		t.Fatal(err)
	}
	called := false
	if err := d.RegisterRequestHandlerAny("/secret", func(string, []byte, []byte, []byte, *identity.Identity, int64) any {
		called = true
		return []byte("leaked")
	}, AllowNone, nil); err != nil {
		t.Fatal(err)
	}
	stranger, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	out := d.HandleRequest("/secret", nil, nil, nil, stranger, 0)
	if called {
		t.Fatal("AllowNone handler was invoked")
	}
	if string(out) == "leaked" {
		t.Fatal("AllowNone returned handler output")
	}
}

func TestBughuntHandleRequestHonorsAllowList(t *testing.T) {
	id, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	allowed, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	d, err := New(id, In, Single, "bughunt", &mockTransport{}, "list")
	if err != nil {
		t.Fatal(err)
	}
	if err := d.RegisterRequestHandlerAny("/x", func(string, []byte, []byte, []byte, *identity.Identity, int64) any {
		return []byte("ok")
	}, AllowList, [][]byte{allowed.Hash()}); err != nil {
		t.Fatal(err)
	}
	stranger, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	if out := d.HandleRequest("/x", nil, nil, nil, stranger, 0); string(out) == "ok" {
		t.Fatal("stranger allowed by AllowList")
	}
	if out := d.HandleRequest("/x", nil, nil, nil, allowed, 0); string(out) != "ok" {
		t.Fatalf("allowed identity rejected: %q", out)
	}
}
