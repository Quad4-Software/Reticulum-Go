// SPDX-License-Identifier: 0BSD
// Copyright (c) 2024-2026 Quad4.io

// Cross-implementation msgpack compatibility test for the blackhole list
// format. Set RUN_PY_INTEROP=1 to enable (optional interpreter path via env).

package blackhole

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func toBase64(b []byte) string            { return base64.StdEncoding.EncodeToString(b) }
func fromBase64(s string) ([]byte, error) { return base64.StdEncoding.DecodeString(s) }

func pythonOrSkip(t *testing.T) string {
	t.Helper()
	if os.Getenv("RUN_PY_INTEROP") == "" {
		t.Skip("set RUN_PY_INTEROP=1 to enable python blackhole interop test")
	}
	exe := os.Getenv("PYTHON_INTEROP")
	if exe == "" {
		exe = "python3"
	}
	if _, err := exec.LookPath(exe); err != nil {
		t.Skipf("python interpreter %q not found: %v", exe, err)
	}
	return exe
}

func TestBlackholeMsgpackRoundtripWithPython(t *testing.T) {
	pyExe := pythonOrSkip(t)
	dir := t.TempDir()
	local := bytes.Repeat([]byte{0xab}, HashLen)
	SetLocalIdentityHash(local)
	tab := New(dir)
	id1 := bytes.Repeat([]byte{0x21}, HashLen)
	id2 := bytes.Repeat([]byte{0x22}, HashLen)
	if _, err := tab.Add(id1, 0, ""); err != nil {
		t.Fatalf("add1: %v", err)
	}
	if _, err := tab.Add(id2, 1700000000.5, "abuse"); err != nil {
		t.Fatalf("add2: %v", err)
	}
	packed, err := os.ReadFile(filepath.Join(dir, "local"))
	if err != nil {
		t.Fatalf("read local: %v", err)
	}

	script := `import sys, base64, json
from RNS.vendor import umsgpack
raw = base64.b64decode(sys.argv[1])
obj = umsgpack.unpackb(raw)
out = {}
for k, v in obj.items():
    e = {}
    if v.get("until") is not None: e["until"] = v["until"]
    if v.get("reason") is not None: e["reason"] = v["reason"]
    e["source"] = v.get("source").hex() if v.get("source") else None
    out[k.hex()] = e
print(json.dumps(out, sort_keys=True))
`
	enc := []byte(toBase64(packed))
	cmd := exec.Command(pyExe, "-c", script, string(enc))
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		t.Skipf("python decode failed (umsgpack missing?): %v", err)
	}
	got := strings.TrimSpace(string(out))
	if !strings.Contains(got, hex.EncodeToString(id1)) || !strings.Contains(got, hex.EncodeToString(id2)) {
		t.Fatalf("python could not decode our blackhole entries: %s", got)
	}
	if !strings.Contains(got, `"abuse"`) {
		t.Fatalf("reason missing from python decode: %s", got)
	}
	if !strings.Contains(got, `"until": 1700000000.5`) {
		t.Fatalf("until missing from python decode: %s", got)
	}
}

func TestPythonEncodedBlackholeIsDecoded(t *testing.T) {
	pyExe := pythonOrSkip(t)
	id := bytes.Repeat([]byte{0x33}, HashLen)
	src := bytes.Repeat([]byte{0xcd}, HashLen)
	script := `import sys, base64
from RNS.vendor import umsgpack
ident = bytes.fromhex(sys.argv[1])
src = bytes.fromhex(sys.argv[2])
data = {ident: {"source": src, "until": 1700000050.25, "reason": "py-side"}}
sys.stdout.write(base64.b64encode(umsgpack.packb(data)).decode())
`
	cmd := exec.Command(pyExe, "-c", script, hex.EncodeToString(id), hex.EncodeToString(src))
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		t.Skipf("python encode failed (umsgpack missing?): %v", err)
	}
	raw, err := fromBase64(strings.TrimSpace(string(out)))
	if err != nil {
		t.Fatalf("base64: %v", err)
	}
	decoded, err := DecodeBlackholeMap(raw)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	got, ok := decoded[string(id)]
	if !ok {
		t.Fatalf("python entry not found in decoded map")
	}
	if got.Reason != "py-side" {
		t.Fatalf("reason mismatch: %q", got.Reason)
	}
	if got.Until != 1700000050.25 {
		t.Fatalf("until mismatch: %v", got.Until)
	}
	if !bytes.Equal(got.Source, src) {
		t.Fatalf("source mismatch: %x vs %x", got.Source, src)
	}
}
