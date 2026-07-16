// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package selfcheck

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/reticulumconfig"
)

func checkDaemon(ctx context.Context, opts Options) Result {
	bin := opts.BinaryPath
	if bin == "" {
		return result(nameDaemonSmoke, SeveritySkip, "BinaryPath not set")
	}
	if _, err := os.Stat(bin); err != nil {
		return result(nameDaemonSmoke, SeverityFail, err.Error())
	}

	dir, err := os.MkdirTemp(opts.WorkDir, "rns-selfcheck-daemon-*")
	if err != nil {
		return result(nameDaemonSmoke, SeverityFail, err.Error())
	}
	defer os.RemoveAll(dir)

	storage := filepath.Join(dir, "storage")
	if err := os.MkdirAll(storage, dirModePrivate); err != nil {
		return result(nameDaemonSmoke, SeverityFail, err.Error())
	}

	ctrlPort, err := freeTCPPort()
	if err != nil {
		return result(nameDaemonSmoke, SeverityFail, err.Error())
	}
	sharedPort, err := freeTCPPort()
	if err != nil {
		return result(nameDaemonSmoke, SeverityFail, err.Error())
	}
	rpcPort, err := freeTCPPort()
	if err != nil {
		return result(nameDaemonSmoke, SeverityFail, err.Error())
	}

	rpcKey := make([]byte, 32)
	if _, err := rand.Read(rpcKey); err != nil {
		return result(nameDaemonSmoke, SeverityFail, err.Error())
	}

	cfgPath := filepath.Join(dir, "config")
	cfg := common.DefaultConfig()
	cfg.ConfigPath = cfgPath
	cfg.EnableSandbox = true
	cfg.EnableSeccomp = true
	// FreeBSD CapEnter and OpenBSD pledge run inside Apply before Control API
	// listen in the daemon. Keep sandbox on for the child probe. Disable it for
	// this daemon smoke so the health endpoint can bind.
	sandboxNote := "sandbox on"
	switch runtime.GOOS {
	case "freebsd", "openbsd":
		cfg.EnableSandbox = false
		cfg.EnableSeccomp = false
		sandboxNote = "sandbox off (" + runtime.GOOS + " CapEnter/pledge before listen)"
	}
	cfg.EnableControlAPI = true
	cfg.ControlAPIHost = "127.0.0.1"
	cfg.ControlAPIPort = ctrlPort
	cfg.ShareInstance = true
	cfg.SharedInstancePort = sharedPort
	cfg.InstanceControlPort = rpcPort
	cfg.RPCKey = rpcKey
	cfg.PanicOnInterfaceErr = false
	cfg.InMemoryStorage = true
	cfg.LogLevel = 3
	cfg.Interfaces = map[string]*common.InterfaceConfig{}
	if err := reticulumconfig.SaveConfig(cfg); err != nil {
		return result(nameDaemonSmoke, SeverityFail, "config: "+err.Error())
	}

	timeout := opts.timeout()
	if timeout > daemonMaxTimeout {
		timeout = daemonMaxTimeout
	}
	dctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(dctx, bin, "-config", dir) // #nosec G204 -- BinaryPath from CLI or CI wrapper
	cmd.Env = append(os.Environ(), "HOME="+dir)
	cmd.Dir = dir
	logPath := filepath.Join(dir, "daemon.log")
	logFile, err := os.Create(logPath) // #nosec G304 -- path under MkdirTemp work dir
	if err != nil {
		return result(nameDaemonSmoke, SeverityFail, err.Error())
	}
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		return result(nameDaemonSmoke, SeverityFail, "start: "+err.Error())
	}

	defer func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
		_ = logFile.Close()
	}()

	url := fmt.Sprintf("http://127.0.0.1:%d/v1/health", ctrlPort)
	token := hex.EncodeToString(rpcKey)
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		if dctx.Err() != nil {
			break
		}
		req, reqErr := http.NewRequestWithContext(dctx, http.MethodGet, url, nil)
		if reqErr != nil {
			lastErr = reqErr
			break
		}
		req.Header.Set("Authorization", "Bearer "+token)
		resp, doErr := http.DefaultClient.Do(req)
		if doErr == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return result(nameDaemonSmoke, SeverityPass,
					fmt.Sprintf("control API healthy (%s, port %d)", sandboxNote, ctrlPort))
			}
			lastErr = fmt.Errorf("health status %d", resp.StatusCode)
		} else {
			lastErr = doErr
		}
		time.Sleep(100 * time.Millisecond)
	}

	logTail := readTail(logPath, logTailLines)
	detail := "control API did not become healthy"
	if lastErr != nil {
		detail += ": " + lastErr.Error()
	}
	if logTail != "" {
		detail += " log=" + logTail
	}
	return result(nameDaemonSmoke, SeverityFail, detail)
}

func freeTCPPort() (int, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()
	return port, nil
}

func readTail(path string, maxLines int) string {
	data, err := os.ReadFile(path) // #nosec G304 -- daemon.log under MkdirTemp work dir
	if err != nil || len(data) == 0 {
		return ""
	}
	lines := splitLines(string(data))
	if len(lines) > maxLines {
		lines = lines[len(lines)-maxLines:]
	}
	out := ""
	for i, line := range lines {
		if i > 0 {
			out += " | "
		}
		if len(line) > logLineMaxChars {
			line = line[:logLineMaxChars] + "..."
		}
		out += line
	}
	if len(out) > logTailMaxChars {
		return out[len(out)-logTailMaxChars:]
	}
	return out
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}
