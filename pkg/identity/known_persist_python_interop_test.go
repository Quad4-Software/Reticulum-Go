// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package identity

import (
	"bytes"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"quad4/msgpack/v5/pkg/msgpack"
	"quad4/reticulum-go/internal/storage"
)

// Guarantee: Go-written known_destinations is accepted by Python
// Identity.load_known_destinations (16-byte keys, 64-byte pubkey values).
func TestKnownDestinationsMsgpackReadableByPython(t *testing.T) {
	py := pythonInteropBin(t)
	resetKnownDestinations(t)
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config")
	InitKnownDestinationsPersistence(cfgPath, false)

	id, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	pkt := []byte("announce-wire")
	app := []byte("app-data")
	if !Remember(pkt, id.Hash(), id.GetPublicKey(), app) {
		t.Fatal("Remember failed")
	}
	SaveKnownDestinationsSync()

	path, err := storage.KnownDestinationsPath(cfgPath)
	if err != nil {
		t.Fatalf("path: %v", err)
	}

	script := `
import sys, binascii, time, os
from RNS.vendor import umsgpack
from RNS.Identity import Identity

path = sys.argv[1]
want = binascii.unhexlify(sys.argv[2])
data = open(path, "rb").read()
loaded = umsgpack.unpackb(data)
if not isinstance(loaded, dict) or len(loaded) != 1:
    raise SystemExit("bad top-level")
key, entry = next(iter(loaded.items()))
if len(key) != 16:
    raise SystemExit(f"key len {len(key)} want 16")
if not isinstance(entry, (list, tuple)) or len(entry) < 4:
    raise SystemExit("bad entry")
pub = entry[2]
if not isinstance(pub, (bytes, bytearray)) or len(pub) != 64:
    raise SystemExit("bad pubkey")
ident = Identity(create_keys=False)
ident.load_public_key(bytes(pub))
if ident.hash != want:
    raise SystemExit("hash mismatch")
# Exercise the same keep filter as Identity.load_known_destinations.
kept = {k: v for k, v in loaded.items() if len(k) == 16}
if len(kept) != 1:
    raise SystemExit(f"kept {len(kept)} want 1")
key_b = key if isinstance(key, (bytes, bytearray)) else key.encode("latin1")
if key_b != want:
    raise SystemExit("kept key mismatch")
print("OK", "kept", len(kept))
`
	cmd := exec.Command(py, "-c", script, path, hex.EncodeToString(id.Hash()))
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("python load known_destinations: %v\n%s", err, out)
	}
	if !bytes.Contains(out, []byte("OK")) || !bytes.Contains(out, []byte("kept 1")) {
		t.Fatalf("unexpected python out: %s", out)
	}

	resetKnownDestinations(t)
	InitKnownDestinationsPersistence(cfgPath, false)
	recalled, err := Recall(id.Hash())
	if err != nil {
		t.Fatalf("Go reload: %v", err)
	}
	if !bytes.Equal(recalled.GetPublicKey(), id.GetPublicKey()) {
		t.Fatal("Go reload pubkey mismatch")
	}
}

// Guarantee: a Python-written byte-keyed known_destinations file still loads
// into Go after the raw-key write change.
func TestKnownDestinationsLoadPythonByteKeysInterop(t *testing.T) {
	py := pythonInteropBin(t)
	resetKnownDestinations(t)
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config")
	InitKnownDestinationsPersistence(cfgPath, false)
	path, err := storage.KnownDestinationsPath(cfgPath)
	if err != nil {
		t.Fatalf("path: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	script := `
import sys, time
from RNS.vendor import umsgpack
from RNS.Identity import Identity
ident = Identity()
dest = ident.hash
entry = [time.time(), b"pkt", ident.get_public_key(), b"app", 0]
umsgpack.dump({dest: entry}, open(sys.argv[1], "wb"))
sys.stdout.buffer.write(dest + ident.get_public_key())
`
	cmd := exec.Command(py, "-c", script, path)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("python write: %v\n%s", err, out)
	}
	if len(out) != 16+64 {
		t.Fatalf("python out len=%d", len(out))
	}
	dest, pub := out[:16], out[16:]

	resetKnownDestinations(t)
	InitKnownDestinationsPersistence(cfgPath, false)
	recalled, err := Recall(dest)
	if err != nil {
		t.Fatalf("Recall python dest: %v", err)
	}
	if !bytes.Equal(recalled.GetPublicKey(), pub) {
		t.Fatalf("pubkey mismatch after loading python file")
	}
	knownDestinationsLock.RLock()
	e := knownDestinations[knownDestKey(dest)]
	knownDestinationsLock.RUnlock()
	if len(e.rawKey) != TruncatedHashLength/8 {
		t.Fatalf("rawKey len=%d want %d", len(e.rawKey), TruncatedHashLength/8)
	}
}

// Guarantee: legacy Go hex-keyed files still load after the write format change.
func TestKnownDestinationsLoadLegacyHexKeys(t *testing.T) {
	resetKnownDestinations(t)
	id, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	destHash := id.Hash()
	export := map[string]any{
		hex.EncodeToString(destHash): []any{
			float64(1),
			[]byte("pkt"),
			id.GetPublicKey(),
			[]byte("app"),
			float64(0),
		},
	}
	data, err := msgpack.Marshal(export)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config")
	writeKnownDestinations(t, cfgPath, data)
	InitKnownDestinationsPersistence(cfgPath, false)
	if _, err := Recall(destHash); err != nil {
		t.Fatalf("Recall legacy hex key: %v", err)
	}
}
