// SPDX-License-Identifier: LicenseRef-Reticulum
package rrc

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
)

type interopResponse struct {
	OK          bool           `json:"ok"`
	Error       string         `json:"error"`
	Trace       string         `json:"trace"`
	RrcdVersion string         `json:"rrcd_version,omitempty"`
	RRCVersion  int            `json:"rrc_version,omitempty"`
	Packed      string         `json:"packed,omitempty"`
	Type        int            `json:"type,omitempty"`
	Version     int            `json:"version,omitempty"`
	Sender      string         `json:"sender,omitempty"`
	MsgID       string         `json:"msg_id,omitempty"`
	Timestamp   uint64         `json:"timestamp,omitempty"`
	Room        string         `json:"room,omitempty"`
	Nick        string         `json:"nick,omitempty"`
	Body        any            `json:"body,omitempty"`
	Envelope    map[string]any `json:"envelope,omitempty"`
	ClientHash  string         `json:"client_hash,omitempty"`
	WelcomeBody map[string]any `json:"welcome_body,omitempty"`
	Text        string         `json:"text,omitempty"`
	Destination string         `json:"destination,omitempty"`
	HubHash     string         `json:"hub_hash,omitempty"`
	PublicKey   string         `json:"public_key,omitempty"`
	ReadyPath   string         `json:"ready_path,omitempty"`
	HubName     string         `json:"hub_name,omitempty"`
	Joined      bool           `json:"joined,omitempty"`
	MsgEcho     bool           `json:"msg_echo,omitempty"`
	Pong        bool           `json:"pong,omitempty"`
	Parted      bool           `json:"parted,omitempty"`
	NoticeOK    bool           `json:"notice_ok,omitempty"`
	ActionOK    bool           `json:"action_ok,omitempty"`
	Constants   map[string]any `json:"constants,omitempty"`
	Normalized  string         `json:"normalized,omitempty"`
	RoundtripOK bool           `json:"roundtrip_ok,omitempty"`
	Packed2     string         `json:"packed2,omitempty"`
	Notices     []string       `json:"notices,omitempty"`
	Errors      []string       `json:"errors,omitempty"`
	Events      []any          `json:"events,omitempty"`
	WelcomeCaps map[string]any `json:"welcome_caps,omitempty"`
	WelcomeLim  map[string]any `json:"welcome_limits,omitempty"`
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
		interopDir = filepath.Join(root, "pkg", "rrc", "interoptest")
		ref := filepath.Join(root, "RRC-ref")
		if _, err := os.Stat(ref); err != nil {
			interopSkipMsg = "RRC-ref clone missing; clone https://github.com/kc1awv/rrcd to RRC-ref"
			return
		}
		if _, err := exec.LookPath("uv"); err != nil {
			interopSkipMsg = "uv not found in PATH"
			return
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
		return "", fmt.Errorf("rrc interop: cannot resolve caller path")
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
		interopDir = filepath.Join(root, "pkg", "rrc", "interoptest")
	}
	payload, err := json.Marshal(req)
	if err != nil {
		return zero, err
	}
	cmd := exec.Command("uv", "run", "python", "harness.py")
	cmd.Dir = interopDir
	cmd.Stdin = bytes.NewReader(payload)
	out, err := cmd.CombinedOutput()
	trimmed := bytes.TrimSpace(out)
	// Harness prints one JSON object as the last non-empty line.
	if idx := bytes.LastIndexByte(trimmed, '\n'); idx >= 0 {
		trimmed = bytes.TrimSpace(trimmed[idx+1:])
	}
	var resp interopResponse
	if len(trimmed) > 0 && json.Unmarshal(trimmed, &resp) == nil && (resp.OK || resp.Error != "") {
		return resp, nil
	}
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return zero, fmt.Errorf("uv run harness: %w: %s", err, string(ee.Stderr))
		}
		return zero, fmt.Errorf("uv run harness: %w: %s", err, string(out))
	}
	if err := json.Unmarshal(trimmed, &resp); err != nil {
		return zero, fmt.Errorf("decode harness output: %w: %s", err, string(out))
	}
	return resp, nil
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
		t.Fatalf("interop: %v", err)
	}
	if !resp.OK {
		t.Fatalf("interop error: %s\n%s", resp.Error, resp.Trace)
	}
	return resp
}

func bodyToJSON(v any) any {
	switch x := v.(type) {
	case []byte:
		return "hex:" + hex.EncodeToString(x)
	case map[uint64]any:
		out := make(map[string]any, len(x))
		for k, val := range x {
			out[fmt.Sprintf("%d", k)] = bodyToJSON(val)
		}
		return out
	case string, float64, bool, nil, int, int64, uint64:
		return x
	default:
		return x
	}
}
