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

func editWithEditor(initial string) (string, bool, error) {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		for _, fb := range []string{"nano", "vim", "vi"} {
			if _, err := exec.LookPath(fb); err == nil {
				editor = fb
				break
			}
		}
	}
	if editor == "" {
		return "", false, fmt.Errorf("no editor found, set EDITOR or use -content")
	}
	tmp, err := os.CreateTemp("", "rnsgit-edit-*")
	if err != nil {
		return "", false, err
	}
	path := tmp.Name()
	defer os.Remove(path)
	if _, err := tmp.WriteString(initial); err != nil {
		tmp.Close()
		return "", false, err
	}
	if err := tmp.Close(); err != nil {
		return "", false, err
	}
	cmd := exec.Command(editor, path) // #nosec G204 -- operator-chosen editor
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
