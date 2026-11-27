// SPDX-License-Identifier: LicenseRef-Reticulum
package lxmf

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"testing"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/destination"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/identity"
)

type interopMessage struct {
	Hash               string         `json:"hash"`
	Title              string         `json:"title"`
	Content            string         `json:"content"`
	Fields             map[string]any `json:"fields"`
	Timestamp          float64        `json:"timestamp"`
	SignatureValidated bool           `json:"signature_validated"`
	UnverifiedReason   any            `json:"unverified_reason"`
	Stamp              string         `json:"stamp"`
	DestinationHash    string         `json:"destination_hash"`
	SourceHash         string         `json:"source_hash"`
}

type interopResponse struct {
	OK    bool   `json:"ok"`
	Error string `json:"error"`
	Trace string `json:"trace"`

	Results []json.RawMessage `json:"results,omitempty"`

	LXMFVersion string `json:"lxmf_version,omitempty"`

	Packed               string         `json:"packed,omitempty"`
	Hash                 string         `json:"hash,omitempty"`
	SourcePublicKey      string         `json:"source_public_key,omitempty"`
	DestinationPublicKey string         `json:"destination_public_key,omitempty"`
	SourceHash           string         `json:"source_hash,omitempty"`
	DestinationHash      string         `json:"destination_hash,omitempty"`
	Message              interopMessage `json:"message"`

	Workblock string  `json:"workblock,omitempty"`
	Valid     bool    `json:"valid,omitempty"`
	Value     float64 `json:"value,omitempty"`
	Stamp     string  `json:"stamp,omitempty"`

	TransientID string `json:"transient_id,omitempty"`
	LxmData     string `json:"lxm_data,omitempty"`

	Container           string `json:"container,omitempty"`
	State               int    `json:"state,omitempty"`
	Method              int    `json:"method,omitempty"`
	TransportEncrypted  bool   `json:"transport_encrypted,omitempty"`
	TransportEncryption string `json:"transport_encryption,omitempty"`

	DisplayName        string `json:"display_name,omitempty"`
	StampCost          any    `json:"stamp_cost,omitempty"`
	CompressionSupport any    `json:"compression_support,omitempty"`
	AppData            string `json:"app_data,omitempty"`

	URI         string `json:"uri,omitempty"`
	PaperPacked string `json:"paper_packed,omitempty"`

	DestHash  string `json:"dest_hash,omitempty"`
	PublicKey string `json:"public_key,omitempty"`
	Text      string `json:"text,omitempty"`
}

var (
	interopOnce    sync.Once
	interopAvail   bool
	interopSkipMsg string
	interopDir     string
)

func interopAvailable() (bool, string) {
	interopOnce.Do(func() {
		root, err := repoRoot()
		if err != nil {
			interopSkipMsg = err.Error()
			return
		}
		interopDir = filepath.Join(root, "pkg", "lxmf", "interoptest")
		if interopPythonPath() == "" {
			ref := filepath.Join(root, "LXMF-ref")
			if _, err := os.Stat(ref); err != nil {
				interopSkipMsg = "python LXMF unavailable; set LXMF_PYTHON or clone upstream LXMF to LXMF-ref"
				return
			}
			if _, err := exec.LookPath("uv"); err != nil {
				interopSkipMsg = "uv not found in PATH"
				return
			}
		}
		resp, err := runInterop(map[string]any{"cmd": "ping"})
		if err != nil {
			interopSkipMsg = err.Error()
			return
		}
		if !resp.OK {
			interopSkipMsg = resp.Error
			if resp.Trace != "" {
				interopSkipMsg += "\n" + resp.Trace
			}
			return
		}
		interopAvail = true
	})
	return interopAvail, interopSkipMsg
}

func repoRoot() (string, error) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "", fmt.Errorf("lxmf interop: cannot resolve caller path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..")), nil
}

func runInterop(req map[string]any) (interopResponse, error) {
	var zero interopResponse
	if interopDir == "" {
		root, err := repoRoot()
		if err != nil {
			return zero, err
		}
		interopDir = filepath.Join(root, "pkg", "lxmf", "interoptest")
	}
	payload, err := json.Marshal(req)
	if err != nil {
		return zero, err
	}
	var cmd *exec.Cmd
	if py := interopPythonPath(); py != "" {
		cmd = exec.Command(py, "harness.py")
	} else {
		cmd = exec.Command("uv", "run", "python", "harness.py")
	}
	cmd.Dir = interopDir
	cmd.Stdin = bytes.NewReader(payload)
	out, err := cmd.CombinedOutput()
	if err != nil {
		var resp interopResponse
		if jsonErr := decodeHarnessOutput(out, &resp); jsonErr == nil && (resp.Error != "" || resp.Trace != "") {
			return resp, fmt.Errorf("%s", resp.Error)
		}
		return zero, fmt.Errorf("uv run harness: %w: %s", err, string(out))
	}
	var resp interopResponse
	if err := decodeHarnessOutput(out, &resp); err != nil {
		return zero, fmt.Errorf("decode harness output: %w: %s", err, string(out))
	}
	return resp, nil
}

func decodeHarnessOutput(out []byte, resp *interopResponse) error {
	out = bytes.TrimSpace(out)
	if len(out) == 0 {
		return fmt.Errorf("empty harness output")
	}
	if out[0] != '{' {
		if i := bytes.LastIndexByte(out, '{'); i >= 0 {
			out = out[i:]
		}
	}
	return json.Unmarshal(out, resp)
}

func interopPythonPath() string {
	if py := os.Getenv("LXMF_PYTHON"); py != "" {
		if _, err := os.Stat(py); err == nil {
			return py
		}
		return ""
	}
	const installed = "/home/dev/.local/share/uv/tools/lxmf/bin/python"
	if _, err := os.Stat(installed); err == nil {
		return installed
	}
	return ""
}

func outDeliveryHash(t *testing.T, id *identity.Identity) []byte {
	t.Helper()
	dest, err := destination.New(id, destination.Out, destination.Single, AppName, nil, "delivery")
	if err != nil {
		t.Fatalf("delivery destination: %v", err)
	}
	return dest.GetHash()
}

func registerInteropSource(t *testing.T, sourceHashHex, publicKeyHex string) {
	t.Helper()
	srcHash, err := hex.DecodeString(sourceHashHex)
	if err != nil {
		t.Fatalf("decode source hash: %v", err)
	}
	pk, err := hex.DecodeString(publicKeyHex)
	if err != nil {
		t.Fatalf("decode public key: %v", err)
	}
	identity.Remember(nil, srcHash, pk, nil)
}

func interopIdentities(t *testing.T, id *identity.Identity) []map[string]string {
	t.Helper()
	return []map[string]string{{
		"hash":       hex.EncodeToString(outDeliveryHash(t, id)),
		"public_key": hex.EncodeToString(id.GetPublicKey()),
	}}
}

func requireInterop(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("interop tests skipped in -short mode")
	}
	ok, msg := interopAvailable()
	if !ok {
		if os.Getenv("CI_REQUIRE_INTEROP") != "" {
			t.Fatalf("interop required in CI: %s", msg)
		}
		t.Skip(msg)
	}
}

func interopCall(t *testing.T, req map[string]any) interopResponse {
	t.Helper()
	resp, err := runInterop(req)
	if err != nil {
		t.Fatalf("interop harness: %v", err)
	}
	if !resp.OK {
		detail := resp.Error
		if resp.Trace != "" {
			detail += "\n" + resp.Trace
		}
		t.Fatalf("interop %v failed: %s", req["cmd"], detail)
	}
	return resp
}
