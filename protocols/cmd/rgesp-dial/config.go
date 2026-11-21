// SPDX-License-Identifier: LicenseRef-Reticulum
package main

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/Quad4-Software/reticulum-go-protocols/pkg/lxst/sandbox"
)

func loadConfigFile(path string) (map[string]string, error) {
	if strings.ContainsRune(path, 0) {
		return nil, fmt.Errorf("invalid path")
	}
	f, err := os.Open(path) // #nosec G304 -- operator-supplied local config path after NUL check
	if err != nil {
		return nil, err
	}
	defer f.Close()
	out := make(map[string]string)
	sc := bufio.NewScanner(f)
	lineNo := 0
	for sc.Scan() {
		lineNo++
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			return nil, fmt.Errorf("%s:%d: expected key = value", path, lineNo)
		}
		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)
		if key == "" {
			return nil, fmt.Errorf("%s:%d: empty key", path, lineNo)
		}
		out[key] = val
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

const maxPort = 65535

func parsePort(name, v string) (int, error) {
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", name, err)
	}
	if n < 1 || n > maxPort {
		return 0, fmt.Errorf("%s out of range", name)
	}
	return n, nil
}

func applyConfigMap(vals map[string]string, set map[string]bool) error {
	for k := range vals {
		if sandbox.ForbiddenConfigKey(k) {
			return fmt.Errorf("%s cannot configure or disable the sandbox", k)
		}
	}
	get := func(name string) (string, bool) {
		if set[name] {
			return "", false
		}
		v, ok := vals[name]
		return v, ok && v != ""
	}
	if err := applyBoolFlag(get, "server", serverMode); err != nil {
		return err
	}
	if v, ok := get("dest"); ok {
		*destHex = v
	}
	if err := applyPortFlag(get, "listen-port", listenPort); err != nil {
		return err
	}
	if err := applyPortFlag(get, "target-port", targetPort); err != nil {
		return err
	}
	if v, ok := get("if"); ok {
		*ifaceKind = v
	}
	if err := applyPortFlag(get, "local-port", localPort); err != nil {
		return err
	}
	if v, ok := get("identity"); ok {
		*identityPath = v
	}
	if err := applyBoolFlag(get, "no-audio", noAudio); err != nil {
		return err
	}
	if v, ok := get("profile"); ok {
		*profileName = v
	}
	if v, ok := get("rnsconfig"); ok {
		*rnsConfig = v
	}
	if v, ok := get("auto-answer"); ok {
		d, err := time.ParseDuration(v)
		if err != nil {
			return fmt.Errorf("auto-answer: %w", err)
		}
		*autoAnswer = d
	}
	return nil
}

func applyPortFlag(get func(string) (string, bool), name string, dst *int) error {
	v, ok := get(name)
	if !ok {
		return nil
	}
	n, err := parsePort(name, v)
	if err != nil {
		return err
	}
	*dst = n
	return nil
}

func applyBoolFlag(get func(string) (string, bool), name string, dst *bool) error {
	v, ok := get(name)
	if !ok {
		return nil
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	*dst = b
	return nil
}
