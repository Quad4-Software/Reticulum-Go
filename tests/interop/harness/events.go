// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package harness

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Event is one structured interop timeline entry.
type Event struct {
	TS     string         `json:"ts"`
	Src    string         `json:"src"`
	Event  string         `json:"event"`
	Kind   string         `json:"kind,omitempty"`
	Detail string         `json:"detail,omitempty"`
	Fields map[string]any `json:"fields,omitempty"`
}

// EventLog appends JSON lines to events.jsonl.
type EventLog struct {
	mu   sync.Mutex
	path string
	all  []Event
}

// NewEventLog creates or truncates events.jsonl under dir.
func NewEventLog(dir string) (*EventLog, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, "events.jsonl")
	f, err := os.Create(path) // #nosec G304 -- dir is ArtifactsDir or TempDir from the test harness
	if err != nil {
		return nil, err
	}
	_ = f.Close()
	return &EventLog{path: path}, nil
}

// Path returns the events.jsonl path.
func (e *EventLog) Path() string {
	if e == nil {
		return ""
	}
	return e.path
}

// Emit appends one event from Go.
func (e *EventLog) Emit(event, kind, detail string, fields map[string]any) {
	e.emit("go", event, kind, detail, fields)
}

func (e *EventLog) emit(src, event, kind, detail string, fields map[string]any) {
	if e == nil {
		return
	}
	ev := Event{
		TS:     time.Now().UTC().Format(time.RFC3339Nano),
		Src:    src,
		Event:  event,
		Kind:   kind,
		Detail: detail,
		Fields: fields,
	}
	b, err := json.Marshal(ev)
	if err != nil {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.all = append(e.all, ev)
	f, err := os.OpenFile(e.path, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	_, _ = f.Write(append(b, '\n'))
	_ = f.Close()
}

// IngestPythonLine parses an INTEROP_EVENT stderr line from a Python peer.
func (e *EventLog) IngestPythonLine(line string) bool {
	const prefix = "INTEROP_EVENT "
	if len(line) < len(prefix) || line[:len(prefix)] != prefix {
		return false
	}
	raw := line[len(prefix):]
	var ev Event
	if err := json.Unmarshal([]byte(raw), &ev); err != nil {
		return false
	}
	if ev.Src == "" {
		ev.Src = "py"
	}
	if ev.TS == "" {
		ev.TS = time.Now().UTC().Format(time.RFC3339Nano)
	}
	b, err := json.Marshal(ev)
	if err != nil {
		return false
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.all = append(e.all, ev)
	f, err := os.OpenFile(e.path, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return true
	}
	_, _ = f.Write(append(b, '\n'))
	_ = f.Close()
	return true
}

// Last returns up to n most recent events.
func (e *EventLog) Last(n int) []Event {
	if e == nil {
		return nil
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if n <= 0 || len(e.all) == 0 {
		return nil
	}
	if n > len(e.all) {
		n = len(e.all)
	}
	out := make([]Event, n)
	copy(out, e.all[len(e.all)-n:])
	return out
}
