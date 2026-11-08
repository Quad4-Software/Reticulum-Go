// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package identity

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/Quad4-Software/Reticulum-Go/internal/storage"
	"github.com/Quad4-Software/msgpack/v5/pkg/msgpack"
)

func writeEntryFile(t *testing.T, cfgPath string, destHash []byte, entry []any) {
	t.Helper()
	path, err := storage.KnownDestinationsPath(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	data, err := marshalKnownDestinationsPython(map[string][]any{string(destHash): entry})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func loadEntry(t *testing.T, cfgPath string, destHash []byte) knownDestEntry {
	t.Helper()
	loadKnownDestinationsFromDisk(cfgPath)
	key := knownDestKey(destHash)
	knownDestinationsLock.RLock()
	e, ok := knownDestinations[key]
	knownDestinationsLock.RUnlock()
	if !ok {
		t.Fatal("entry not loaded")
	}
	return e
}

func TestKnownDestinationsLoadLegacyLengths(t *testing.T) {
	for _, n := range []int{5, 6, 7, 8} {
		resetKnownDestinations(t)
		cfgPath := filepath.Join(t.TempDir(), "config")
		id, err := New()
		if err != nil {
			t.Fatal(err)
		}
		pub := id.GetPublicKey()
		lr := 7.5
		entry := []any{float64(1700000000), []byte("pkt"), pub, []byte("app"), float64(0)}
		if n >= 6 {
			entry = append(entry, []byte("12345678901234"))
		}
		if n >= 7 {
			entry = append(entry, float64(9.25))
		}
		if n >= 8 {
			entry = append(entry, lr)
		}
		writeEntryFile(t, cfgPath, id.Hash(), entry)
		e := loadEntry(t, cfgPath, id.Hash())

		if n >= 6 {
			if !bytes.Equal(e.stamp, []byte("12345678901234")) {
				t.Fatalf("n=%d stamp=%x", n, e.stamp)
			}
		} else if e.stamp != nil {
			t.Fatalf("n=%d unexpected stamp", n)
		}
		if n >= 7 {
			if e.stampValue != 9.25 {
				t.Fatalf("n=%d stampValue=%v", n, e.stampValue)
			}
		} else if e.stampValue != 0 {
			t.Fatalf("n=%d stampValue=%v", n, e.stampValue)
		}
		if n >= 8 {
			if e.lrCost == nil || *e.lrCost != 7.5 {
				t.Fatalf("n=%d lrCost=%v", n, e.lrCost)
			}
		} else if e.lrCost != nil {
			t.Fatalf("n=%d unexpected lrCost", n)
		}
	}
}

func TestKnownDestinationsEightFieldRoundTrip(t *testing.T) {
	t.Setenv("RETICULUM_DISABLE_SQLITE_KNOWN", "1")
	resetKnownDestinations(t)
	cfgPath := filepath.Join(t.TempDir(), "config")
	InitKnownDestinationsPersistence(cfgPath, false)

	id, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer id.Close()
	lr := 33.25
	info := &StampInfo{Stamp: []byte("12345678901234"), Value: 11.5, LRCost: &lr}
	if !RememberStamped([]byte("pkt"), id.Hash(), id.GetPublicKey(), []byte("app"), id, info) {
		t.Fatal("RememberStamped failed")
	}
	SaveKnownDestinationsSync()

	path, err := storage.KnownDestinationsPath(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var loaded map[string]any
	if err := msgpack.Unmarshal(data, &loaded); err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 1 {
		t.Fatalf("entries=%d", len(loaded))
	}
	for _, raw := range loaded {
		entry := raw.([]any)
		if len(entry) != 8 {
			t.Fatalf("entry len=%d, want 8 (python 1.6.0)", len(entry))
		}
		if !bytes.Equal(entry[5].([]byte), []byte("12345678901234")) {
			t.Fatal("stamp mismatch")
		}
		if entry[6].(float64) != 11.5 {
			t.Fatal("stamp_value mismatch")
		}
		if entry[7].(float64) != 33.25 {
			t.Fatal("lr_cost mismatch")
		}
	}

	resetKnownDestinations(t)
	InitKnownDestinationsPersistence(cfgPath, false)
	if v, ok := RecallStampValue(id.Hash()); !ok || v != 11.5 {
		t.Fatalf("reloaded stamp value %v ok=%v", v, ok)
	}
	if c, ok := RecallLinkCost(id.Hash()); !ok || c != 33.25 {
		t.Fatalf("reloaded lr cost %v ok=%v", c, ok)
	}
}

// Guarantee: a Go-written 8-field file is accepted by Python 1.6.0's legacy
// known_destinations load path (umsgpack + entry upgrade semantics).
func TestKnownDestinationsEightFieldReadableByPython(t *testing.T) {
	t.Setenv("RETICULUM_DISABLE_SQLITE_KNOWN", "1")
	py := pythonInteropBin(t)
	resetKnownDestinations(t)
	cfgPath := filepath.Join(t.TempDir(), "config")
	InitKnownDestinationsPersistence(cfgPath, false)

	id, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer id.Close()
	lr := 21.75
	info := &StampInfo{Stamp: []byte("12345678901234"), Value: 17.25, LRCost: &lr}
	if !RememberStamped([]byte("pkt"), id.Hash(), id.GetPublicKey(), []byte("app"), id, info) {
		t.Fatal("RememberStamped failed")
	}
	SaveKnownDestinationsSync()

	path, err := storage.KnownDestinationsPath(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	script := `
import sys
from RNS.vendor import umsgpack
data = open(sys.argv[1], "rb").read()
loaded = umsgpack.unpackb(data)
key, entry = next(iter(loaded.items()))
if len(entry) != 8:
    raise SystemExit(f"entry len {len(entry)} want 8")
if entry[5] != b"12345678901234":
    raise SystemExit("stamp mismatch")
if entry[6] != 17.25:
    raise SystemExit(f"stamp_value {entry[6]}")
if entry[7] != 21.75:
    raise SystemExit(f"lr_cost {entry[7]}")
print("OK")
`
	cmd := exec.Command(py, "-c", script, path)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("python load: %v\n%s", err, out)
	}
	if !bytes.Contains(out, []byte("OK")) {
		t.Fatalf("unexpected python out: %s", out)
	}
}

// Guarantee: a Python-1.6.0-written 8-field entry loads stamp fields into Go.
func TestKnownDestinationsLoadPythonEightField(t *testing.T) {
	t.Setenv("RETICULUM_DISABLE_SQLITE_KNOWN", "1")
	py := pythonInteropBin(t)
	resetKnownDestinations(t)
	cfgPath := filepath.Join(t.TempDir(), "config")
	InitKnownDestinationsPersistence(cfgPath, false)
	path, err := storage.KnownDestinationsPath(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	script := `
import sys, time
from RNS.vendor import umsgpack
from RNS.Identity import Identity
ident = Identity()
dest = ident.hash
stamp = bytes(range(14))
entry = [time.time(), b"pkt", ident.get_public_key(), b"app", 0, stamp, 13.5, 22.75]
umsgpack.dump({dest: entry}, open(sys.argv[1], "wb"))
sys.stdout.buffer.write(dest + ident.get_public_key())
`
	cmd := exec.Command(py, "-c", script, path)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("python write: %v\n%s", err, out)
	}
	if len(out) != 80 {
		t.Fatalf("python out len=%d", len(out))
	}
	dest, pub := out[:16], out[16:]

	resetKnownDestinations(t)
	InitKnownDestinationsPersistence(cfgPath, false)
	recalled, err := Recall(dest)
	if err != nil {
		t.Fatalf("Recall: %v", err)
	}
	if !bytes.Equal(recalled.GetPublicKey(), pub) {
		t.Fatal("pubkey mismatch")
	}
	if v, ok := RecallStampValue(dest); !ok || v != 13.5 {
		t.Fatalf("stamp value %v ok=%v", v, ok)
	}
	if c, ok := RecallLinkCost(dest); !ok || c != 22.75 {
		t.Fatalf("lr cost %v ok=%v", c, ok)
	}
}
