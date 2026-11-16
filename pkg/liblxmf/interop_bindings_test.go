// SPDX-License-Identifier: LicenseRef-Reticulum

package liblxmf_test

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/identity"
	"github.com/Quad4-Software/reticulum-go-protocols/pkg/lxmf"
)

type bindingsUnpackResponse struct {
	OK         bool           `json:"ok"`
	Error      string         `json:"error"`
	Content    string         `json:"content"`
	Title      string         `json:"title"`
	FieldCount int            `json:"field_count"`
	Fields     map[string]any `json:"fields"`
}

func TestBindingsInterop_GoPackPythonUnpack(t *testing.T) {
	if testing.Short() {
		t.Skip("bindings interop skipped in -short mode")
	}
	python := bindingsPython(t)
	client := filepath.Join(repoRootBindings(t), "bindings", "python", "interoptest", "unpack_client.py")

	src, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	dst, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	packed, err := lxmf.PackMessagingInterop(src, dst, true)
	if err != nil {
		t.Fatal(err)
	}
	srcHash, err := lxmf.DeliveryHash(src)
	if err != nil {
		t.Fatal(err)
	}

	req, err := json.Marshal(map[string]any{
		"cmd":         "unpack",
		"packed":      hex.EncodeToString(packed),
		"source_hash": hex.EncodeToString(srcHash),
		"public_key":  hex.EncodeToString(src.GetPublicKey()),
	})
	if err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(python, client)
	cmd.Dir = filepath.Join(repoRootBindings(t), "bindings", "python")
	cmd.Env = append(os.Environ(),
		"PYTHONPATH=.",
		"LXMF_LIB_PATH="+filepath.Join(repoRootBindings(t), "bin", "liblxmf.so"),
	)
	cmd.Stdin = bytes.NewReader(req)
	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			t.Fatalf("python unpack client: %v: %s", err, string(ee.Stderr))
		}
		t.Fatal(err)
	}

	var resp bindingsUnpackResponse
	if err := json.Unmarshal(bytes.TrimSpace(out), &resp); err != nil {
		t.Fatalf("decode: %v: %s", err, string(out))
	}
	if !resp.OK {
		t.Fatalf("unpack failed: %s", resp.Error)
	}
	if resp.Content != "messaging fields" {
		t.Fatalf("content=%q", resp.Content)
	}
	if resp.Title != "interop" {
		t.Fatalf("title=%q", resp.Title)
	}
	wantKeys := len(lxmf.MessagingInteropFieldKeys(true))
	if resp.FieldCount < wantKeys {
		t.Fatalf("field_count=%d want>=%d", resp.FieldCount, wantKeys)
	}
	for _, id := range lxmf.MessagingInteropFieldKeys(true) {
		key := formatFieldKey(id)
		if _, ok := resp.Fields[key]; !ok {
			t.Fatalf("missing field %s", key)
		}
	}
}

func TestBindingsInterop_PythonPackGoUnpack(t *testing.T) {
	if testing.Short() {
		t.Skip("bindings interop skipped in -short mode")
	}
	python := bindingsPython(t)
	client := filepath.Join(repoRootBindings(t), "bindings", "python", "interoptest", "unpack_client.py")
	fieldsPath := filepath.Join(repoRootBindings(t), "pkg", "lxmf", "testdata", "messaging_interop_fields.json")
	fieldsJSON, err := os.ReadFile(fieldsPath)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := json.Unmarshal(fieldsJSON, &fields); err != nil {
		t.Fatal(err)
	}

	req, err := json.Marshal(map[string]any{
		"cmd":     "pack",
		"title":   "bindings",
		"content": "from python bindings",
		"fields":  fields,
	})
	if err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(python, client)
	cmd.Dir = filepath.Join(repoRootBindings(t), "bindings", "python")
	cmd.Env = append(os.Environ(),
		"PYTHONPATH=.",
		"LXMF_LIB_PATH="+filepath.Join(repoRootBindings(t), "bin", "liblxmf.so"),
	)
	cmd.Stdin = bytes.NewReader(req)
	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			t.Fatalf("python pack client: %v: %s", err, string(ee.Stderr))
		}
		t.Fatal(err)
	}

	var packResp struct {
		OK         bool   `json:"ok"`
		Error      string `json:"error"`
		Packed     string `json:"packed"`
		SourceHash string `json:"source_hash"`
		PublicKey  string `json:"public_key"`
	}
	if err := json.Unmarshal(bytes.TrimSpace(out), &packResp); err != nil {
		t.Fatalf("decode: %v: %s", err, string(out))
	}
	if !packResp.OK {
		t.Fatalf("pack failed: %s", packResp.Error)
	}

	srcHash, err := hex.DecodeString(packResp.SourceHash)
	if err != nil {
		t.Fatal(err)
	}
	publicKey, err := hex.DecodeString(packResp.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	identity.Remember(nil, srcHash, publicKey, nil)

	packed, err := hex.DecodeString(packResp.Packed)
	if err != nil {
		t.Fatal(err)
	}
	got, err := lxmf.Unpack(packed, lxmf.RecallSource)
	if err != nil {
		t.Fatal(err)
	}
	if !got.SignatureValidated {
		t.Fatal("signature not validated")
	}
	if got.ContentString() != "from python bindings" {
		t.Fatalf("content=%q", got.ContentString())
	}
	assertGoFieldKeysBindings(t, got.Fields, lxmf.MessagingInteropFieldKeys(false))
}

func assertGoFieldKeysBindings(t *testing.T, fields map[byte]any, want []byte) {
	t.Helper()
	if fields == nil {
		t.Fatal("fields nil")
	}
	for _, id := range want {
		if _, ok := fields[id]; !ok {
			t.Fatalf("missing field 0x%02x", id)
		}
	}
}

func formatFieldKey(id byte) string {
	return fmt.Sprintf("0x%02x", id)
}

func repoRootBindings(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve caller")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func bindingsPython(t *testing.T) string {
	t.Helper()
	if p := os.Getenv("PYTHON"); p != "" {
		return p
	}
	if p, err := exec.LookPath("python3"); err == nil {
		return p
	}
	t.Skip("python3 not found")
	return ""
}
