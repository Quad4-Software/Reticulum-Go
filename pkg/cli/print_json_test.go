// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/rnsutil"
)

func writePrintTestConfig(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	content := "[reticulum]\nenable_transport = no\nshare_instance = no\n\n[interfaces]\n"
	if err := os.WriteFile(filepath.Join(dir, "config"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func preparePrintTestIdentity(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, "identity")
	if _, err := rnsutil.GenerateIdentity(path); err != nil {
		t.Fatal(err)
	}
	return path
}

func runPrintJSON(t *testing.T, args ...string) (map[string]any, int) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := Main(args, Options{Stdout: &stdout, Stderr: &stderr, VersionLine: "test"})
	if code != 0 {
		t.Fatalf("args=%v code=%d stderr=%s stdout=%s", args, code, stderr.String(), stdout.String())
	}
	var out map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(stdout.Bytes()), &out); err != nil {
		t.Fatalf("args=%v invalid JSON %q: %v", args, stdout.String(), err)
	}
	return out, code
}

func TestPrintIdentityJSONShellTools(t *testing.T) {
	for _, cmd := range []string{"sh", "cp", "x"} {
		t.Run(cmd, func(t *testing.T) {
			dir := writePrintTestConfig(t)
			idPath := preparePrintTestIdentity(t, dir)
			out, _ := runPrintJSON(t, cmd, "-config", dir, "-i", idPath, "-p", "-json")
			identityHex, ok := out["identity"].(string)
			if !ok || len(identityHex) != 32 {
				t.Fatalf("identity value: %#v", out["identity"])
			}
			if cmd == "sh" {
				dests, ok := out["destinations"].(map[string]any)
				if !ok {
					t.Fatalf("destinations missing: %#v", out)
				}
				for _, key := range []string{"rgosh", "rnsh"} {
					if s, ok := dests[key].(string); !ok || len(s) != 32 {
						t.Fatalf("destination %s: %#v", key, dests[key])
					}
				}
				return
			}
			if s, ok := out["destination"].(string); !ok || len(s) != 32 {
				t.Fatalf("destination: %#v", out["destination"])
			}
		})
	}
}

func TestPrintIdentityJSONID(t *testing.T) {
	dir := t.TempDir()
	idPath := preparePrintTestIdentity(t, dir)
	out, _ := runPrintJSON(t, "id", "-i", idPath, "-p", "-json")
	if s, ok := out["identity"].(string); !ok || len(s) != 32 {
		t.Fatalf("identity: %#v", out["identity"])
	}
	if s, ok := out["public_key"].(string); !ok || len(s) != 128 {
		t.Fatalf("public_key: %#v", out["public_key"])
	}
	if _, ok := out["private_key"]; ok {
		t.Fatal("private key present without -P")
	}
	out, _ = runPrintJSON(t, "id", "-i", idPath, "-p", "-json", "-P")
	if s, ok := out["private_key"].(string); !ok || len(s) != 128 {
		t.Fatalf("private_key with -P: %#v", out["private_key"])
	}
}
