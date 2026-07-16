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
	"syscall"
	"time"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/reticulumconfig"
	"quad4/reticulum-go/pkg/rnsutil"
)

const (
	nameDaemonRPC    = "daemon/rpc-smoke"
	nameDaemonReload = "daemon/reload"
	reloadIfaceName  = "selfcheck_reload_udp"
)

func checkDaemon(ctx context.Context, opts Options) []Result {
	bin := opts.BinaryPath
	if bin == "" {
		return []Result{
			result(nameDaemonSmoke, SeveritySkip, "BinaryPath not set"),
			result(nameDaemonRPC, SeveritySkip, "BinaryPath not set"),
			result(nameDaemonReload, SeveritySkip, "BinaryPath not set"),
		}
	}
	absBin, err := filepath.Abs(bin)
	if err != nil {
		fail := result(nameDaemonSmoke, SeverityFail, "binary path: "+err.Error())
		return []Result{fail, result(nameDaemonRPC, SeveritySkip, "daemon not started"), result(nameDaemonReload, SeveritySkip, "daemon not started")}
	}
	bin = absBin
	if _, err := os.Stat(bin); err != nil {
		fail := result(nameDaemonSmoke, SeverityFail, err.Error())
		return []Result{fail, result(nameDaemonRPC, SeveritySkip, "daemon not started"), result(nameDaemonReload, SeveritySkip, "daemon not started")}
	}

	dir, err := os.MkdirTemp(opts.WorkDir, "rns-selfcheck-daemon-*")
	if err != nil {
		fail := result(nameDaemonSmoke, SeverityFail, err.Error())
		return []Result{fail, result(nameDaemonRPC, SeveritySkip, "setup failed"), result(nameDaemonReload, SeveritySkip, "setup failed")}
	}
	defer os.RemoveAll(dir)

	storage := filepath.Join(dir, "storage")
	if err := os.MkdirAll(storage, dirModePrivate); err != nil {
		fail := result(nameDaemonSmoke, SeverityFail, err.Error())
		return []Result{fail, result(nameDaemonRPC, SeveritySkip, "setup failed"), result(nameDaemonReload, SeveritySkip, "setup failed")}
	}

	ctrlPort, err := freeTCPPort()
	if err != nil {
		fail := result(nameDaemonSmoke, SeverityFail, err.Error())
		return []Result{fail, result(nameDaemonRPC, SeveritySkip, "setup failed"), result(nameDaemonReload, SeveritySkip, "setup failed")}
	}
	sharedPort, err := freeTCPPort()
	if err != nil {
		fail := result(nameDaemonSmoke, SeverityFail, err.Error())
		return []Result{fail, result(nameDaemonRPC, SeveritySkip, "setup failed"), result(nameDaemonReload, SeveritySkip, "setup failed")}
	}
	rpcPort, err := freeTCPPort()
	if err != nil {
		fail := result(nameDaemonSmoke, SeverityFail, err.Error())
		return []Result{fail, result(nameDaemonRPC, SeveritySkip, "setup failed"), result(nameDaemonReload, SeveritySkip, "setup failed")}
	}

	rpcKey := make([]byte, 32)
	if _, err := rand.Read(rpcKey); err != nil {
		fail := result(nameDaemonSmoke, SeverityFail, err.Error())
		return []Result{fail, result(nameDaemonRPC, SeveritySkip, "setup failed"), result(nameDaemonReload, SeveritySkip, "setup failed")}
	}

	cfgPath := filepath.Join(dir, "config")
	cfg := common.DefaultConfig()
	cfg.ConfigPath = cfgPath
	cfg.EnableSandbox = true
	cfg.EnableSeccomp = true
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
		fail := result(nameDaemonSmoke, SeverityFail, "config: "+err.Error())
		return []Result{fail, result(nameDaemonRPC, SeveritySkip, "setup failed"), result(nameDaemonReload, SeveritySkip, "setup failed")}
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
		fail := result(nameDaemonSmoke, SeverityFail, err.Error())
		return []Result{fail, result(nameDaemonRPC, SeveritySkip, "setup failed"), result(nameDaemonReload, SeveritySkip, "setup failed")}
	}
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		fail := result(nameDaemonSmoke, SeverityFail, "start: "+err.Error())
		return []Result{fail, result(nameDaemonRPC, SeveritySkip, "daemon not started"), result(nameDaemonReload, SeveritySkip, "daemon not started")}
	}

	defer func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
		_ = logFile.Close()
	}()

	out := make([]Result, 0, 3)
	smoke := waitControlAPI(dctx, ctrlPort, rpcKey, timeout, logPath)
	out = append(out, smoke)
	if smoke.Severity == SeverityFail {
		out = append(out, result(nameDaemonRPC, SeveritySkip, "control API not healthy"))
		out = append(out, result(nameDaemonReload, SeveritySkip, "control API not healthy"))
		return out
	}

	out = append(out, checkDaemonRPC(dctx, cfg, rpcKey, timeout))
	out = append(out, checkDaemonReload(dctx, cmd, cfg, rpcKey, timeout, logPath))
	return out
}

func waitControlAPI(ctx context.Context, ctrlPort int, rpcKey []byte, timeout time.Duration, logPath string) Result {
	url := fmt.Sprintf("http://127.0.0.1:%d/v1/health", ctrlPort)
	token := hex.EncodeToString(rpcKey)
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		if ctx.Err() != nil {
			break
		}
		req, reqErr := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
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
					fmt.Sprintf("control API healthy (sandbox on, port %d)", ctrlPort))
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

func checkDaemonRPC(ctx context.Context, cfg *common.ReticulumConfig, rpcKey []byte, timeout time.Duration) Result {
	client, err := rnsutil.DialRPC(cfg, rpcKey)
	if err != nil {
		return result(nameDaemonRPC, SeverityFail, "dial: "+err.Error())
	}
	client.SetTimeout(timeout)
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		if ctx.Err() != nil {
			break
		}
		stats, err := client.GetInterfaceStats()
		if err == nil {
			return result(nameDaemonRPC, SeverityPass,
				fmt.Sprintf("GetInterfaceStats ok (interfaces=%d uptime=%.0fs)", len(stats.Interfaces), stats.TransportUptime))
		}
		lastErr = err
		time.Sleep(100 * time.Millisecond)
	}
	detail := "GetInterfaceStats failed"
	if lastErr != nil {
		detail += ": " + lastErr.Error()
	}
	return result(nameDaemonRPC, SeverityFail, detail)
}

func checkDaemonReload(ctx context.Context, cmd *exec.Cmd, cfg *common.ReticulumConfig, rpcKey []byte, timeout time.Duration, logPath string) Result {
	if runtime.GOOS == "windows" {
		return result(nameDaemonReload, SeveritySkip, "SIGHUP not used on windows")
	}
	if runtime.GOOS == "freebsd" {
		// CapEnter after startup blocks opening the config file and new sockets.
		return result(nameDaemonReload, SeveritySkip, "CapEnter blocks post-sandbox reload opens")
	}

	ln, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		return result(nameDaemonReload, SeverityFail, err.Error())
	}
	udpPort := ln.LocalAddr().(*net.UDPAddr).Port
	_ = ln.Close()

	cfg.Interfaces = map[string]*common.InterfaceConfig{
		reloadIfaceName: {
			Type:       "UDPInterface",
			Enabled:    true,
			Address:    fmt.Sprintf("127.0.0.1:%d", udpPort),
			TargetHost: "127.0.0.1",
			TargetPort: 9,
		},
	}
	if err := reticulumconfig.SaveConfig(cfg); err != nil {
		return result(nameDaemonReload, SeverityFail, "rewrite config: "+err.Error())
	}

	if err := cmd.Process.Signal(syscall.SIGHUP); err != nil {
		return result(nameDaemonReload, SeverityFail, "SIGHUP: "+err.Error())
	}

	client, err := rnsutil.DialRPC(cfg, rpcKey)
	if err != nil {
		return result(nameDaemonReload, SeverityFail, "dial: "+err.Error())
	}
	client.SetTimeout(timeout)
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		if ctx.Err() != nil {
			break
		}
		stats, err := client.GetInterfaceStats()
		if err != nil {
			lastErr = err
			time.Sleep(100 * time.Millisecond)
			continue
		}
		for _, st := range stats.Interfaces {
			if st.Name == reloadIfaceName || st.ShortName == reloadIfaceName {
				return result(nameDaemonReload, SeverityPass,
					fmt.Sprintf("SIGHUP loaded %s", reloadIfaceName))
			}
		}
		if len(stats.Interfaces) > 0 {
			return result(nameDaemonReload, SeverityPass,
				fmt.Sprintf("SIGHUP reload visible (%d interfaces)", len(stats.Interfaces)))
		}
		lastErr = fmt.Errorf("interface %s not listed yet", reloadIfaceName)
		time.Sleep(100 * time.Millisecond)
	}

	detail := "reload did not expose new interface"
	if lastErr != nil {
		detail += ": " + lastErr.Error()
	}
	if tail := readTail(logPath, logTailLines); tail != "" {
		detail += " log=" + tail
	}
	return result(nameDaemonReload, SeverityFail, detail)
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
