// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package rgosh

import "testing"

func TestEscapeQuit(t *testing.T) {
	e := NewEscapeFilter()
	out, act := e.Feed([]byte("~."))
	if act != EscapeQuit {
		t.Fatalf("action=%d", act)
	}
	if len(out) != 0 {
		t.Fatalf("out=%q", out)
	}
}

func TestEscapeLiteralTilde(t *testing.T) {
	e := NewEscapeFilter()
	out, act := e.Feed([]byte("~~x"))
	if act != EscapeNone {
		t.Fatalf("action=%d", act)
	}
	if string(out) != "~x" {
		t.Fatalf("out=%q", out)
	}
}

func TestEscapeAfterNewline(t *testing.T) {
	e := NewEscapeFilter()
	out, act := e.Feed([]byte("hi\r~L"))
	if act != EscapeToggleLine {
		t.Fatalf("action=%d out=%q", act, out)
	}
	if string(out) != "hi\r" {
		t.Fatalf("out=%q", out)
	}
}

func TestEscapeNotMidLine(t *testing.T) {
	e := NewEscapeFilter()
	out, act := e.Feed([]byte("a~.b"))
	if act != EscapeNone {
		t.Fatalf("action=%d", act)
	}
	if string(out) != "a~.b" {
		t.Fatalf("out=%q", out)
	}
}
