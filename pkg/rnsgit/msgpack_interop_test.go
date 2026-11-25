// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package rnsgit

import (
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestListRequestMsgpackPythonDecode(t *testing.T) {
	b, err := EncodeMixedRequest(map[any]any{IdxRepository: "public/demo"})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("hex: %s", hex.EncodeToString(b))
	_, testFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(testFile), "..", ".."))
	python := "python3"
	if venvPython := filepath.Join(root, ".venv", "bin", "python"); fileExists(venvPython) {
		python = venvPython
	}
	cmd := exec.Command(python, "-c", `
import sys
import RNS.vendor.umsgpack as umsgpack
data = umsgpack.unpackb(bytes.fromhex(sys.argv[1]))
print(type(data).__name__)
for k, v in data.items():
    print(repr(k), type(v).__name__, repr(v))
`, hex.EncodeToString(b))
	if python == "python3" {
		cmd.Env = append(os.Environ(), "PYTHONPATH="+filepath.Join(root, "reticulum-ref"))
	}
	out, err := cmd.CombinedOutput()
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

func fileExists(path string) bool {
	st, err := os.Stat(path)
	return err == nil && !st.IsDir()
}
