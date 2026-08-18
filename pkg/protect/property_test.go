// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

//go:build !haiku

package protect

import (
	"bytes"
	"testing"
	"testing/quick"
	"time"
)

func TestPropertyDetectNeverBlocks(t *testing.T) {
	f := func(n uint8, size uint16) bool {
		var buf bytes.Buffer
		clock := time.Unix(1_700_000_000, 0)
		e := New(Options{
			Mode:            ModeDetect,
			MaxPPS:          3,
			MaxBPS:          100,
			WarnWriter:      &buf,
			WarnInterval:    time.Hour,
			DisableAdaptive: true,
			DisableCoolDown: true,
			Now:             func() time.Time { return clock },
		})
		for i := 0; i < int(n)+1; i++ {
			d := e.AdmitPacket("pbt", int(size)+1)
			if !d.Allow {
				return false
			}
		}
		return true
	}
	if err := quick.Check(f, &quick.Config{MaxCount: 200}); err != nil {
		t.Fatal(err)
	}
}

func TestPropertyPreventBlocksOnlyAfterThreshold(t *testing.T) {
	f := func(extra uint8) bool {
		var buf bytes.Buffer
		clock := time.Unix(1_700_000_000, 0)
		const maxPPS = 5.0
		e := New(Options{
			Mode:            ModePrevent,
			MaxPPS:          maxPPS,
			WarnWriter:      &buf,
			WarnInterval:    time.Hour,
			DisableAdaptive: true,
			DisableCoolDown: true,
			Now:             func() time.Time { return clock },
		})
		for range int(maxPPS) {
			d := e.AdmitPacket("pbt", 1)
			if !d.Allow {
				return false
			}
		}
		for i := 0; i < int(extra)+1; i++ {
			d := e.admitWithOpts("pbt", 1, AdmitOpts{Class: ClassShedFirst})
			if d.Allow {
				return false
			}
		}
		return true
	}
	if err := quick.Check(f, &quick.Config{MaxCount: 100}); err != nil {
		t.Fatal(err)
	}
}

func TestPropertyWarnAtMostOnePerWindow(t *testing.T) {
	f := func(n uint8) bool {
		var buf bytes.Buffer
		clock := time.Unix(1_700_000_000, 0)
		e := New(Options{
			Mode:            ModeDetect,
			MaxPPS:          1,
			WarnWriter:      &buf,
			WarnInterval:    time.Second,
			DisableAdaptive: true,
			DisableCoolDown: true,
			Now:             func() time.Time { return clock },
		})
		for i := 0; i < int(n)+2; i++ {
			_ = e.AdmitPacket("w", 1)
		}
		lines := bytes.Count(buf.Bytes(), []byte("\n"))
		return lines == 1
	}
	if err := quick.Check(f, &quick.Config{MaxCount: 50}); err != nil {
		t.Fatal(err)
	}
}

func TestPBTModeRoundTrip(t *testing.T) {
	for _, m := range []Mode{ModeOff, ModeDetect, ModePrevent, ModeAuto} {
		got, ok := ParseMode(m.String())
		if !ok || got != m {
			t.Fatalf("round trip %v", m)
		}
	}
}

func TestPropertyCryptoReleaseMonotonic(t *testing.T) {
	f := func(n uint8) bool {
		var buf bytes.Buffer
		e := New(Options{Mode: ModePrevent, MaxCrypto: 3, WarnWriter: &buf, WarnInterval: time.Hour})
		var releases []func()
		allowed := 0
		for i := 0; i < int(n)%10+4; i++ {
			d, rel := e.AdmitCrypto("c")
			if d.Allow {
				allowed++
				releases = append(releases, rel)
			}
		}
		if allowed > 3 {
			return false
		}
		for _, rel := range releases {
			rel()
		}
		d, rel := e.AdmitCrypto("c")
		rel()
		return d.Allow
	}
	if err := quick.Check(f, &quick.Config{MaxCount: 80}); err != nil {
		t.Fatal(err)
	}
}
