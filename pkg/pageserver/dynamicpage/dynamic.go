// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

// Package dynamicpage implements executable .mu pages (shebang + execute bit),
// matching rns-page-node serve_page semantics.
package dynamicpage

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"quad4/msgpack/v5/pkg/msgpack"

	"quad4/reticulum-go/pkg/debug"
	"quad4/reticulum-go/pkg/identity"
)

func shebangLine(content []byte) bool {
	line := content
	if before, _, ok := bytes.Cut(content, []byte{'\n'}); ok {
		line = before
	}
	return bytes.HasPrefix(bytes.TrimSpace(line), []byte("#!"))
}

func fileExecutable(fi os.FileInfo) bool {
	mode := fi.Mode()
	if !mode.IsRegular() {
		return false
	}
	return mode.Perm()&0111 != 0
}

func appendEnvironFromData(base []string, data []byte) []string {
	if len(data) == 0 {
		return base
	}
	var m map[string]any
	if err := msgpack.Unmarshal(data, &m); err != nil {
		return base
	}
	out := base
	for k, v := range m {
		if !strings.HasPrefix(k, "field_") && !strings.HasPrefix(k, "var_") {
			continue
		}
		out = append(out, k+"="+fmt.Sprint(v))
	}
	return out
}

func buildScriptEnv(data []byte, linkID []byte, remoteIdentity *identity.Identity) []string {
	base := os.Environ()
	env := make([]string, len(base), len(base)+8)
	copy(env, base)
	env = appendEnvironFromData(env, data)
	if len(linkID) > 0 {
		env = append(env, "link_id="+hex.EncodeToString(linkID))
	}
	if remoteIdentity != nil {
		env = append(env, "remote_identity="+hex.EncodeToString(remoteIdentity.Hash()))
	}
	return env
}

// ReadOrExecute returns static .mu bytes, or stdout from the page script when
// the file is .mu, starts with a shebang, and has an execute bit set.
func ReadOrExecute(filePath string, data []byte, linkID []byte, remoteIdentity *identity.Identity) ([]byte, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		return nil, err
	}

	raw, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}

	if !strings.EqualFold(filepath.Ext(filePath), ".mu") {
		return raw, nil
	}
	if !shebangLine(raw) {
		return raw, nil
	}
	if !fileExecutable(fi) {
		return raw, nil
	}

	cmd := exec.Command(filePath)
	cmd.Env = buildScriptEnv(data, linkID, remoteIdentity)
	out, err := cmd.Output()
	if err != nil {
		debug.Log(debug.DebugError, "dynamic .mu page execution failed. Serving file contents", "path", filePath, "error", err)
		return raw, nil
	}
	return out, nil
}
