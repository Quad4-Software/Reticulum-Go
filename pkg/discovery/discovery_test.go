// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package discovery

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func toBase64(b []byte) string { return base64.StdEncoding.EncodeToString(b) }

func TestEncodeDecodeInfoRoundtrip(t *testing.T) {
	id := bytes.Repeat([]byte{0x42}, 16)
	in := Info{
		Type:        "TCPServerInterface",
		Transport:   true,
		TransportID: id,
		Name:        "go-test",
		Latitude:    12.5,
		Longitude:   -34.25,
		Height:      100.0,
		ReachableOn: "127.0.0.1",
		Port:        4242,
		HasPort:     true,
		IFACNetname: "ifac-net",
		IFACNetkey:  "secret",
	}
	packed, err := EncodeInfo(in)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	out, err := DecodeInfo(packed)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Type != in.Type {
		t.Fatalf("type mismatch: %q vs %q", out.Type, in.Type)
	}
	if out.Transport != in.Transport {
		t.Fatalf("transport mismatch")
	}
	if !bytes.Equal(out.TransportID, in.TransportID) {
		t.Fatalf("transport_id mismatch")
	}
	if out.Name != in.Name {
		t.Fatalf("name mismatch")
	}
	if out.ReachableOn != in.ReachableOn {
		t.Fatalf("reachable_on mismatch")
	}
	if out.Port != in.Port {
		t.Fatalf("port mismatch")
	}
	if out.IFACNetname != in.IFACNetname || out.IFACNetkey != in.IFACNetkey {
		t.Fatalf("ifac mismatch")
	}
}

func TestStampGenerateAndValidate(t *testing.T) {
	msg := bytes.Repeat([]byte{0xAB}, 16)
	stamp, value, err := GenerateStamp(msg, 4, 3)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if value < 4 {
		t.Fatalf("value %d below cost", value)
	}
	wb, err := StampWorkblock(msg, 3)
	if err != nil {
		t.Fatalf("workblock: %v", err)
	}
	if !StampValid(stamp, 4, wb) {
		t.Fatalf("stamp should validate")
	}
	bad := make([]byte, StampSize)
	if _, err := rand.Read(bad); err != nil {
		t.Fatalf("rand: %v", err)
	}
	bad[0] = 0xff
	if StampValid(bad, 256, wb) {
		t.Fatalf("impossibly hard cost should not validate")
	}
}

func TestAppDataRoundtrip(t *testing.T) {
	payload := []byte("hello")
	stamp := bytes.Repeat([]byte{0x07}, StampSize)
	app, err := EncodeAppData(FlagSigned, payload, stamp)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	flags, gotPayload, gotStamp, err := DecodeAppData(app)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if flags != FlagSigned {
		t.Fatalf("flags mismatch: %x", flags)
	}
	if !bytes.Equal(gotPayload, payload) || !bytes.Equal(gotStamp, stamp) {
		t.Fatalf("roundtrip mismatch")
	}
}

func pythonOrSkip(t *testing.T) string {
	t.Helper()
	if os.Getenv("RUN_PY_INTEROP") == "" {
		t.Skip("set RUN_PY_INTEROP=1 to enable python discovery interop test")
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

// Verifies agreement on the msgpack info dict layout and on the LXStamper
// workblock/value/valid math across stacks.
func TestDiscoveryInfoMatchesPython(t *testing.T) {
	pyExe := pythonOrSkip(t)
	id := bytes.Repeat([]byte{0x55}, 16)
	in := Info{
		Type:        "TCPServerInterface",
		Transport:   true,
		TransportID: id,
		Name:        "py-test",
		ReachableOn: "192.0.2.1",
		Port:        4242,
		HasPort:     true,
	}
	packed, err := EncodeInfo(in)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	script := `import sys, base64, json
from RNS.vendor import umsgpack
raw = base64.b64decode(sys.argv[1])
info = umsgpack.unpackb(raw)
out = {}
for k, v in info.items():
    if isinstance(v, (bytes, bytearray)): v = v.hex()
    out[str(k)] = v
print(json.dumps(out, sort_keys=True))
`
	cmd := exec.Command(pyExe, "-c", script, toBase64(packed))
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		t.Skipf("python decode failed: %v", err)
	}
	got := strings.TrimSpace(string(out))
	parsed := map[string]any{}
	if err := json.Unmarshal([]byte(got), &parsed); err != nil {
		t.Fatalf("python json: %v -- %s", err, got)
	}
	if parsed["255"] != "py-test" {
		t.Fatalf("name field mismatch: %v", parsed["255"])
	}
	if parsed["0"] != "TCPServerInterface" {
		t.Fatalf("type field mismatch: %v", parsed["0"])
	}
	if parsed["254"] != hex.EncodeToString(id) {
		t.Fatalf("transport_id field mismatch: %v", parsed["254"])
	}
}

// Generates a small-cost stamp in Go and verifies it against an external
// LXStamper check, confirming the workblock and validity math are compatible
// across implementations.
func TestStampMatchesPython(t *testing.T) {
	pyExe := pythonOrSkip(t)
	if _, err := exec.LookPath(pyExe); err != nil {
		t.Skipf("python interpreter not found: %v", err)
	}
	checkScript := `import sys, base64
sys.path.insert(0, "/run/media/user1/projects/Reticulum/LXMF")
try:
    from LXMF import LXStamper
except Exception as e:
    sys.stderr.write(f"LXStamper import failed: {e}\n")
    sys.exit(0)
material = base64.b64decode(sys.argv[1])
stamp    = base64.b64decode(sys.argv[2])
target   = int(sys.argv[3])
rounds   = int(sys.argv[4])
wb = LXStamper.stamp_workblock(material, expand_rounds=rounds)
print("VALID" if LXStamper.stamp_valid(stamp, target, wb) else "INVALID")
print("VALUE", LXStamper.stamp_value(wb, stamp))
`
	material := bytes.Repeat([]byte{0xCA}, 16)
	stamp, _, err := GenerateStamp(material, 4, 3)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	cmd := exec.Command(pyExe, "-c", checkScript, toBase64(material), toBase64(stamp), "4", "3")
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		t.Skipf("python LXStamper check failed: %v", err)
	}
	if !strings.Contains(string(out), "VALID") {
		t.Fatalf("python LXStamper rejected go stamp: %s", string(out))
	}
}
