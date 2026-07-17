// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package destination

import (
	"bytes"
	"strings"
	"testing"

	"quad4/pbt/pkg/pbt"
	"quad4/reticulum-go/pkg/identity"
)

// FuzzParseName ensures ParseName never panics and rejects empty names.
func FuzzParseName(f *testing.F) {
	f.Add("")
	f.Add("app")
	f.Add("app.aspect")
	f.Add("app.aspect.extra")
	f.Add("...")
	f.Add("   ")
	f.Add(strings.Repeat("a", 256))

	f.Fuzz(func(t *testing.T, name string) {
		if len(name) > 1<<12 {
			t.Skip()
		}
		app, aspects, err := ParseName(name)
		if strings.TrimSpace(name) == "" {
			if err == nil {
				t.Fatal("empty name must error")
			}
			return
		}
		if err != nil {
			return
		}
		if app == "" {
			t.Fatal("non-empty parse must yield app name")
		}
		joined := app
		if len(aspects) > 0 {
			joined = app + "." + strings.Join(aspects, ".")
		}
		if joined != strings.TrimSpace(name) {
			t.Fatalf("roundtrip mismatch got %q want %q", joined, strings.TrimSpace(name))
		}
	})
}

func TestPBTHashStableForSameInputs(t *testing.T) {
	id, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	name := pbt.Map(
		"name",
		pbt.StringASCII(1, 32),
		func(s string) string {
			s = strings.Map(func(r rune) rune {
				if r == '.' || r == ' ' {
					return 'x'
				}
				return r
			}, s)
			if s == "" {
				return "a"
			}
			return s
		},
	)
	prop := pbt.ForAll(
		"destination hash deterministic",
		name,
		func(app string) bool {
			h1 := Hash(id, app)
			h2 := Hash(id, app)
			return len(h1) == 16 && bytes.Equal(h1, h2)
		},
	)
	pbt.Check(t, prop, pbt.WithRuns(80), pbt.WithSeed(21))
}

func TestProofStrategyAndCallbackGetters(t *testing.T) {
	id, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	d, err := New(id, In, Single, "app", &mockTransport{})
	if err != nil {
		t.Fatal(err)
	}
	if d.ProofStrategy() != ProveNone {
		t.Fatalf("default ProofStrategy = %d, want ProveNone", d.ProofStrategy())
	}
	called := false
	cb := func(hash []byte, appData []byte) { called = true }
	d.SetProofRequestedCallback(cb)
	d.SetProofStrategy(ProveApp)
	if d.ProofStrategy() != ProveApp {
		t.Fatalf("ProofStrategy = %d, want ProveApp", d.ProofStrategy())
	}
	got := d.ProofRequestedCallback()
	if got == nil {
		t.Fatal("ProofRequestedCallback nil after set")
	}
	got(nil, nil)
	if !called {
		t.Fatal("callback not invoked")
	}
}
