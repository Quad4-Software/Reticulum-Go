// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package rnsgit

import (
	"encoding/hex"
	"os/exec"
	"strings"
	"testing"
)

func TestListRequestMsgpackPythonDecode(t *testing.T) {
	b, err := EncodeMixedRequest(map[any]any{IdxRepository: "public/demo"})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("hex: %s", hex.EncodeToString(b))
	out, err := exec.Command("python3", "-c", `
import sys
sys.path.insert(0, "")
import RNS.vendor.umsgpack as umsgpack
data = umsgpack.unpackb(bytes.fromhex(sys.argv[1]))
print(type(data).__name__)
for k, v in data.items():
    print(repr(k), type(v).__name__, repr(v))
`, hex.EncodeToString(b)).CombinedOutput()
	if err != nil {
		t.Fatalf("python: %v %s", err, out)
	}
	s := string(out)
	if !strings.Contains(s, "public/demo") {
		t.Fatalf("unexpected python decode:\n%s", s)
	}
	if strings.Contains(s, "bytes") && strings.Contains(s, "public/demo") {
		t.Fatalf("repository path decoded as bytes:\n%s", s)
	}
}
