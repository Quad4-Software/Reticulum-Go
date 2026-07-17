// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package harness

import (
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// ArtifactsDir decides where interop debug files go for this test.
func ArtifactsDir(t *testing.T) string {
	t.Helper()
	base := strings.TrimSpace(os.Getenv("INTEROP_ARTIFACT_ROOT"))
	if base == "" {
		if AlwaysArtifacts() {
			base = filepath.Join(os.TempDir(), "reticulum-interop-artifacts", sanitizeName(t.Name()))
			_ = os.MkdirAll(base, 0o700)
			return base
		}
		base = t.TempDir()
		return base
	}
	base = filepath.Join(base, sanitizeName(t.Name()))
	_ = os.MkdirAll(base, 0o700) // #nosec G703 -- path under INTEROP_ARTIFACT_ROOT plus sanitized test name
	return base
}

func sanitizeName(name string) string {
	r := strings.NewReplacer("/", "_", "\\", "_", " ", "_")
	return r.Replace(name)
}

// AlwaysArtifacts reports whether INTEROP_ARTIFACTS=1.
func AlwaysArtifacts() bool {
	return os.Getenv("INTEROP_ARTIFACTS") == "1"
}

// EventsEnabled reports whether event logging should run.
func EventsEnabled() bool {
	if os.Getenv("INTEROP_EVENTS") == "1" {
		return true
	}
	return AlwaysArtifacts()
}

// WriteEnvSnapshot writes selected env keys for failure triage.
func WriteEnvSnapshot(dir string, extra map[string]string) error {
	keys := []string{
		"RUN_LIVE_INTEROP",
		"PYTHON_INTEROP",
		"INTEROP_ARTIFACTS",
		"INTEROP_EVENTS",
		"INTEROP_LISTEN_PORT",
		"INTEROP_FORWARD_PORT",
		"INTEROP_GO_DEST_HASH",
		"INTEROP_NOMADNET_DEST_HASH",
	}
	m := make(map[string]string, len(keys)+len(extra))
	for _, k := range keys {
		if v := os.Getenv(k); v != "" {
			m[k] = v
		}
	}
	maps.Copy(m, extra)
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "env.json"), b, 0o600)
}

// WriteStderrCapture saves captured probe stderr.
func WriteStderrCapture(dir, content string) error {
	return os.WriteFile(filepath.Join(dir, "stderr.txt"), []byte(content), 0o600)
}

// LogArtifacts summarizes the artifact dir and last events on failure.
func LogArtifacts(t *testing.T, dir string, events *EventLog) {
	t.Helper()
	t.Logf("interop artifacts at %s", dir)
	if events == nil {
		return
	}
	last := events.Last(20)
	for _, ev := range last {
		t.Logf("event src=%s name=%s kind=%s detail=%s", ev.Src, ev.Event, ev.Kind, ev.Detail)
	}
}

// RecordPorts stores listen and forward ports in the env snapshot.
func RecordPorts(extra map[string]string, listen, forward int) map[string]string {
	if extra == nil {
		extra = make(map[string]string)
	}
	extra["INTEROP_LISTEN_PORT"] = strconv.Itoa(listen)
	extra["INTEROP_FORWARD_PORT"] = strconv.Itoa(forward)
	return extra
}
