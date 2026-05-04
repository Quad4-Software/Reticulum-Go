// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package main

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"git.quad4.io/Go-Libs/msgpack/v5/pkg/msgpack"

	"git.quad4.io/Networks/Reticulum-Go/pkg/debug"
	"git.quad4.io/Networks/Reticulum-Go/pkg/identity"
)

func dynamicPageShebangLine(content []byte) bool {
	line := content
	if i := bytes.IndexByte(content, '\n'); i >= 0 {
		line = content[:i]
	}
	return bytes.HasPrefix(bytes.TrimSpace(line), []byte("#!"))
}

func dynamicPageExecutable(fi os.FileInfo) bool {
	mode := fi.Mode()
	if !mode.IsRegular() {
		return false
	}
	return mode.Perm()&0111 != 0
}

func appendDynamicPageEnvironFromData(base []string, data []byte) []string {
	if len(data) == 0 {
		return base
	}
	var m map[string]interface{}
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

func buildDynamicPageScriptEnv(data []byte, linkID []byte, remoteIdentity *identity.Identity) []string {
	env := append([]string(nil), os.Environ()...)
	env = appendDynamicPageEnvironFromData(env, data)
	if len(linkID) > 0 {
		env = append(env, "link_id="+hex.EncodeToString(linkID))
	}
	if remoteIdentity != nil {
		env = append(env, "remote_identity="+hex.EncodeToString(remoteIdentity.Hash()))
	}
	return env
}

// readOrExecuteDynamicPage returns static .mu bytes, or stdout from the page
// script when the file is .mu, starts with a shebang, and has an execute bit
// (same rules as rns-page-node serve_page).
func readOrExecuteDynamicPage(filePath string, data []byte, linkID []byte, remoteIdentity *identity.Identity) ([]byte, error) {
	fi, err := os.Stat(filePath)
	if err != nil {
		return nil, err
	}

	raw, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	if !strings.EqualFold(filepath.Ext(filePath), ".mu") {
		return raw, nil
	}
	if !dynamicPageShebangLine(raw) {
		return raw, nil
	}
	if !dynamicPageExecutable(fi) {
		return raw, nil
	}

	cmd := exec.Command(filePath)
	cmd.Env = buildDynamicPageScriptEnv(data, linkID, remoteIdentity)
	out, err := cmd.Output()
	if err != nil {
		debug.Log(debug.DebugError, "dynamic .mu page execution failed; serving file contents", "path", filePath, "error", err)
		return raw, nil
	}
	return out, nil
}
