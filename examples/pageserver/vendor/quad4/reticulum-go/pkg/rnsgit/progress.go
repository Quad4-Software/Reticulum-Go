// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package rnsgit

import (
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"os"
	"os/exec"
	"strings"
)

func (c *Client) emitProgress(event string, fields map[string]any) {
	if !c.jsonProgress || c.progressWriter == nil {
		return
	}
	m := map[string]any{"event": event}
	maps.Copy(m, fields)
	b, err := json.Marshal(m)
	if err != nil {
		return
	}
	fmt.Fprintln(c.progressWriter, string(b))
}

func statusLine(w io.Writer, msg string) {
	if w == nil {
		return
	}
	fmt.Fprint(w, msg)
}

func statusLinef(w io.Writer, format string, args ...any) {
	if w == nil {
		return
	}
	fmt.Fprintf(w, format, args...)
}

func resolveEditor() (string, error) {
	if ed := strings.TrimSpace(os.Getenv("EDITOR")); ed != "" {
		name := strings.Fields(ed)[0]
		path, err := exec.LookPath(name)
		if err != nil {
			return "", fmt.Errorf("EDITOR %q not executable: %w", name, err)
		}
		return path, nil
	}
	for _, fb := range []string{"nano", "vim", "vi"} {
		if path, err := exec.LookPath(fb); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("no editor found, set EDITOR or use -content")
}

func editWithEditor(initial string) (string, bool, error) {
	editor, err := resolveEditor()
	if err != nil {
		return "", false, err
	}
	tmp, err := os.CreateTemp("", "rnsgit-edit-*")
	if err != nil {
		return "", false, err
	}
	path := tmp.Name()
	defer os.Remove(path)
	if _, err := tmp.WriteString(initial); err != nil {
		if closeErr := tmp.Close(); closeErr != nil {
			return "", false, fmt.Errorf("write temp file: %w (close: %v)", err, closeErr)
		}
		return "", false, err
	}
	if err := tmp.Close(); err != nil {
		return "", false, err
	}
	cmd := exec.Command(editor, path) // #nosec G204 -- editor from exec.LookPath, path from os.CreateTemp
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return "", false, err
	}
	out, err := os.ReadFile(path) // #nosec G304 -- temp editor file
	if err != nil {
		return "", false, err
	}
	return string(out), true, nil
}
